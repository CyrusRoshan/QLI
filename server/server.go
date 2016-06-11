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

	"github.com/cyrusroshan/qli/utils"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/kennygrant/sanitize"
)

const (
	MsgDontSendFile = "Hash recieved for song, no need to send full file."
	MsgSendFullFile = "Song hash not found, send full base64 encoded file."
	MsgSongQueued   = "Song successfully added to queue."
	MsgOnlyMP3      = "File rejected. Please only send MP3 files."
)

var (
	ErrNoSongsLeft = errors.New("There are no songs left in queue.")
)

var CurrentlyPlaying = false
var SongQueue = make(map[string]*UserData)
var HashHolder = make(map[string]string)
var DownloadFolder, _ = filepath.Abs("./serverSongs")

type ServerStruct struct {
	Port *int
}

type SongHolder struct {
	IpAddr     string `json:"ipAddr"`
	FileName   string `json:"fileName"`
	FileHash   string `json:"fileHash"`
	FileURL    string `json:"fileURL"`
	FileSearch string `json:"fileSearch"`
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

	ipAddr, err := ServerAddress()
	utils.PanicIf(err)
	log.Println("Tell your friends to send requests to " + ipAddr + ":" + strconv.Itoa(*server.Port))
	HashExistingFiles(DownloadFolder)

	r := mux.NewRouter()
	r.HandleFunc("/checkHash", CheckHash).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/uploadFile", UploadFile).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/getQueue", GetQueue).
		Methods("GET")

	r.NotFoundHandler = http.HandlerFunc(ErrorPage)

	portString := strconv.Itoa(*server.Port)
	http.ListenAndServe(":"+portString, handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(r))
}

func HashExistingFiles(folder string) {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		err := os.Mkdir(folder, 0777)
		utils.PanicIf(err)
		return
	}
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
			HashHolder[string(hash[:])] = songName
		}
	}

	log.Println("Hashed existing serverSongs folder contents, which was " + strconv.Itoa(totalMp3s) + " total .mp3 files")
}

func CheckHash(w http.ResponseWriter, r *http.Request) {
	sentHash, err := ioutil.ReadAll(r.Body)
	utils.PanicIf(err)

	var msg string
	songName, exists := HashHolder[string(sentHash)]
	if exists {
		msg = MsgDontSendFile
		w.WriteHeader(200)
		go QueueSong(SongHolder{ClientAddress(r), songName, string(sentHash), "", ""})
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
	file, err := os.Create(DownloadFolder + "/" + cleanFileName + ".mp3")
	utils.PanicIf(err)
	defer file.Close()
	_, err = file.Write(decodedRawData)
	utils.PanicIf(err)
	file.Sync()

	QueueSong(SongHolder{ClientAddress(r), cleanFileName, string(hash[:]), "", ""})

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
		HashHolder[song.FileHash] = song.FileName
	}

	_, userExists := SongQueue[song.IpAddr]
	if !userExists {
		SongQueue[song.IpAddr] = &UserData{false, nil}
	}

	SongQueue[song.IpAddr].Songs = append(SongQueue[song.IpAddr].Songs, song)
	log.Println(song.FileName + " successfully added to queue. (ip: " + song.IpAddr + ")")
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
	if nextSong.FileHash != "" {
		log.Println("CURRENTLY PLAYING " + nextSong.FileName)
		playSong := exec.Command("afplay", DownloadFolder+"/"+nextSong.FileName)
		fmt.Println(DownloadFolder + "/" + nextSong.FileName)
		err := playSong.Run()
		if err != nil {
			log.Println(err)
		}
		log.Println("DONE PLAYING " + nextSong.FileName)
	}
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
	ipAddr := r.RemoteAddr[:len(r.RemoteAddr)-6]
	if ipAddr == "[::1]" {
		ipAddr = "localhost"
	}
	return ipAddr
}

func ErrorPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(403)
	w.Write([]byte("Page access forbidden with current credentials."))
}
