package server

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cyrusroshan/qli/utils"
	"github.com/fatih/color"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/kennygrant/sanitize"
)

var (
	CurrentlyPlaying = false
	SongQueue        = make(map[string]*UserData)
	HashHolder       = make(map[string]string)

	DownloadFolder, _ = filepath.Abs("./serverSongs")
	RawMp3Folder, _   = filepath.Abs("./serverSongs/rawmp3")
	YoutubeFolder, _  = filepath.Abs("./serverSongs/youtube")
	SpotifyFolder, _  = filepath.Abs("./serverSongs/spotify")

	infoMsg  = color.New(color.FgYellow).Add(color.Bold)
	musicMsg = color.New(color.FgBlue).Add(color.Bold)
)

const (
	MsgDontSendFile = "Hash recieved for song, no need to send full file."
	MsgSendFullFile = "Song hash not found, send full base64 encoded file."
	MsgSongQueued   = "Song successfully added to queue."
	MsgOnlyMP3      = "File rejected. Please only send MP3 files."
)

const (
	RAWMP3 = iota
	YOUTUBE
	SPOTIFY
	SEARCH
)

var (
	ErrNoSongsLeft = errors.New("There are no songs left in queue.")
)

type ServerStruct struct {
	Port *int
}

type SongHolder struct {
	IpAddr   string `json:"ipAddr"`
	Name     string `json:"fileName"`
	Type     int    `json:"songType"`
	FileHash string `json:"fileHash"`
	URL      string `json:"fileURL"`
	Search   string `json:"fileSearch"`
}

type SongData struct {
	FileName string `json:"fileName"`
	Data     string `json:"data"`
}

type UserData struct {
	Played bool         `json:"played"`
	Songs  []SongHolder `json:"songs"`
}

func StartServer(server ServerStruct) {
	log.Println("Starting qli server")

	utils.MakeDirIfNotExist(DownloadFolder)
	utils.MakeDirIfNotExist(RawMp3Folder)
	utils.MakeDirIfNotExist(YoutubeFolder)
	utils.MakeDirIfNotExist(SpotifyFolder)

	HashExistingFiles(RawMp3Folder)

	ipAddr, err := ServerAddress()
	utils.PanicIf(err)
	infoMsg.Println("Tell your friends to send requests to " + ipAddr + ":" + strconv.Itoa(*server.Port))

	utils.PanicIf(err)

	r := mux.NewRouter()
	r.HandleFunc("/checkHash", CheckHash).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/uploadFile", UploadFile).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/queueURL", QueueURL).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/getQueue", GetQueue).
		Methods("GET")

	r.NotFoundHandler = http.HandlerFunc(ErrorPage)

	portString := strconv.Itoa(*server.Port)
	http.ListenAndServe(":"+portString, handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(r))
}

func HashExistingFiles(folder string) {
	utils.MakeDirIfNotExist(folder)
	files, _ := ioutil.ReadDir(folder)

	totalMp3s := 0
	for _, file := range files {
		songName := file.Name()
		if songName[len(songName)-4:] == ".mp3" {
			log.Println("Hashing: " + songName)
			totalMp3s++
			dat, err := ioutil.ReadFile(folder + "/" + songName)
			utils.PanicIf(err)
			hash := md5.Sum(dat)
			HashHolder[string(hash[:])] = songName[:len(songName)-4]
		}
	}

	folderName := strings.Split(folder, "/")
	log.Println("Hashed existing " + folderName[len(folderName)-1] + " files, which was " + strconv.Itoa(totalMp3s) + " total .mp3 files")
}

func CheckHash(w http.ResponseWriter, r *http.Request) {
	sentHash, err := ioutil.ReadAll(r.Body)
	utils.PanicIf(err)

	var msg string
	songName, exists := HashHolder[string(sentHash)]
	if exists {
		msg = MsgDontSendFile
		w.WriteHeader(200)
		go QueueSong(SongHolder{
			IpAddr:   ClientAddress(r),
			Name:     songName,
			Type:     RAWMP3,
			FileHash: string(sentHash),
			URL:      "",
			Search:   "",
		})
	} else {
		msg = MsgSendFullFile
		w.WriteHeader(404)
	}

	log.Println(msg + " (ip: " + ClientAddress(r) + ")")
	w.Write([]byte(utils.ToJSON(msg)))
}

func UploadFile(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var song SongData
	err := json.NewDecoder(r.Body).Decode(&song)
	utils.PanicIf(err)

	name := song.FileName[:len(song.FileName)-4]
	extension := song.FileName[len(song.FileName)-4:]
	cleanFileName := sanitize.BaseName(name)

	log.Println("Recieved request to queue " + name)
	if extension != ".mp3" {
		w.WriteHeader(200)
		log.Println(name + " rejected. Not an .mp3 file. (ip: " + ClientAddress(r) + ")")
		w.Write([]byte(utils.ToJSON(MsgOnlyMP3)))
		return
	}

	decodedRawData, err := base64.StdEncoding.DecodeString(song.Data)
	utils.PanicIf(err)
	hash := md5.Sum(decodedRawData)
	songName, exists := HashHolder[string(hash[:])]

	if exists {
		msg := (`Recieved raw data for "` + name + `", aka "` + songName + `". Request rejected, send hashes before full song data.`)
		w.WriteHeader(403)
		log.Println(msg + " (ip: " + ClientAddress(r) + ")")
		w.Write([]byte(msg))
		return
	}
	file, err := os.Create(RawMp3Folder + "/" + cleanFileName + ".mp3")
	utils.PanicIf(err)
	defer file.Close()
	_, err = file.Write(decodedRawData)
	utils.PanicIf(err)
	file.Sync()

	go QueueSong(SongHolder{
		IpAddr:   ClientAddress(r),
		Name:     cleanFileName,
		Type:     RAWMP3,
		FileHash: string(hash[:]),
		URL:      "",
		Search:   "",
	})

	w.WriteHeader(200)
	w.Write([]byte(utils.ToJSON(MsgSongQueued)))
}

func QueueURL(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var song SongHolder
	err := json.NewDecoder(r.Body).Decode(&song)
	utils.PanicIf(err)

	song.IpAddr = ClientAddress(r)

	if song.Type == YOUTUBE {
		song.Name = song.URL[len(song.URL)-11:]
		if _, err := os.Stat(YoutubeFolder + "/" + song.Name + ".mp3"); os.IsNotExist(err) {
			log.Println("DOWNLOADING", song.Name)
			youtube := exec.Command("youtube-dl", "--extract-audio", "--audio-format=mp3", "--audio-quality=0", "--output="+YoutubeFolder+"/"+song.Name+".%(ext)s", song.URL)
			err := youtube.Run()
			utils.PanicIf(err)
			log.Println("DOWNLOADED", song.Name)
		} else {
			log.Println("PREVIOUSLY DOWNLOADED, ADDING TO QUEUE", song.Name)
		}
		go QueueSong(song)
	}

	w.WriteHeader(200)
	w.Write([]byte(utils.ToJSON(MsgSongQueued)))
}

func GetQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte(utils.ToJSON(SongQueue)))
}

func QueueSong(song SongHolder) {
	if song.FileHash != "" {
		HashHolder[song.FileHash] = song.Name
	}

	_, userExists := SongQueue[song.IpAddr]
	if !userExists {
		SongQueue[song.IpAddr] = &UserData{false, nil}
	}

	SongQueue[song.IpAddr].Songs = append(SongQueue[song.IpAddr].Songs, song)
	log.Println(song.Name + " successfully added to queue. (ip: " + song.IpAddr + ")")
	if !CurrentlyPlaying {
		PlaySongs()
	}
}

func PlaySongs() {
	CurrentlyPlaying = true
	nextSong, err := PopQueue()
	if err != nil {
		log.Println(err)
		CurrentlyPlaying = false
		return
	}

	musicMsg.Println("CURRENTLY PLAYING " + nextSong.Name)

	var folder string
	switch nextSong.Type {
	case RAWMP3:
		folder = RawMp3Folder
	case YOUTUBE:
		folder = YoutubeFolder
	case SPOTIFY:
		folder = SpotifyFolder
	default:
		panic(errors.New(fmt.Sprintf("%d is not a recognized song type.", nextSong.Type)))
	}

	color.Set(color.FgYellow)
	songPlayer := exec.Command("mpg123", fmt.Sprintf("%s/%s.mp3", folder, nextSong.Name))
	songPlayer.Stdout = os.Stdout
	songPlayer.Stdin = os.Stdin
	songPlayer.Stderr = os.Stderr
	err = songPlayer.Run()
	color.Unset()
	if err != nil {
		log.Println(err)
	}

	log.Println("DONE PLAYING " + nextSong.Name)
	PlaySongs()
}

func PopQueue() (SongHolder, error) {
	// pretty ugly, but it's like 10 people max and I'm lazy
	var songsExist bool
	for user, _ := range SongQueue {
		if !SongQueue[user].Played && len(SongQueue[user].Songs) > 0 {
			SongQueue[user].Played = true
			poppedSong := SongQueue[user].Songs[0]
			SongQueue[user].Songs = SongQueue[user].Songs[1:]
			return poppedSong, nil
		}
		if len(SongQueue[user].Songs) > 0 {
			songsExist = true
		}
	}
	for user, _ := range SongQueue {
		SongQueue[user].Played = false
	}
	if songsExist {
		return PopQueue()
	}
	return SongHolder{}, ErrNoSongsLeft
}

func ServerAddress() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, a := range addrs {
		if ipnet, exists := a.(*net.IPNet); exists && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", errors.New("Local IP address not found")
}

func ClientAddress(r *http.Request) string {
	ipAddr := r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	if ipAddr == "[::1]" {
		ipAddr = "localhost"
	}
	return ipAddr
}

func ErrorPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(403)
	w.Write([]byte("Page access forbidden with current credentials."))
}
