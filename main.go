package main

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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/kennygrant/sanitize"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	MsgDontSendFile = "Hash recieved for song, no need to send full file."
	MsgSendFullFile = "Song hash not found, send full base64 encoded file."
	MsgSongQueued   = "Song successfully added to queue."
)

type serverStruct struct {
	port *int
}

type clientStruct struct {
	serverURL  **url.URL
	songFile   **os.File
	songURL    **url.URL
	songSearch *string
}

type songHolder struct {
	ipAddr     string
	fileName   string
	fileURL    string
	fileSearch string
}

type SongData struct {
	FileName string `json:"filename"`
	Data     string `json:"data"`
}

var songQueue []songHolder
var hashHolder = make(map[string]string)

var downloadFolder, _ = filepath.Abs("./serverSongs")

func main() {
	app := kingpin.New("qli", "queuemmand line interface to queue music between friends")

	serverMode := app.Command("server", "Start qli in server mode.")
	server := serverStruct{
		serverMode.Arg("port", "Port to run qli server on.").Default("3005").Int(),
	}

	clientMode := app.Command("client", "Start qli in client mode")
	client := clientStruct{
		clientMode.Arg("server", "qli server IP address and port number.").Required().URL(),
		clientMode.Flag("file", "Play an mp3 file on the server.").Short('f').File(),
		clientMode.Flag("url", "Play music from a youtube or spotify url.").Short('u').URL(),
		clientMode.Flag("search", "Search for a song on spotify and select one to be played.").Short('s').String(),
	}

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	// Register user
	case serverMode.FullCommand():
		startServer(server)

	case clientMode.FullCommand():
		if *client.songFile != nil {
			queueFile(client)
		} else if *client.songURL != nil {
			queueURL(client)
		} else if *client.songSearch != "" {
			queueSearch(client)
		} else {
			fmt.Println("Please enter a file, url, or song to search, in order to queuemmand a song")
		}
	}
}

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}

func toJSON(data interface{}) string {
	jsonString, err := json.Marshal(data)
	panicIf(err)
	return string(jsonString)
}

//*******************
// Client code here
//*******************

func queueFile(client clientStruct) {
	songPath := (*client.songFile).Name()
	songName := filepath.Base((*client.songFile).Name())
	dat, err := ioutil.ReadFile(songPath)
	panicIf(err)

	hash := md5.Sum(dat)

	var hashResponse string
	sendData(client, "checkHash", string(hash[:]), &hashResponse)

	if hashResponse != MsgSendFullFile {
		fmt.Println(hashResponse)
		return
	}

	encodedData := base64.StdEncoding.EncodeToString(dat)
	song := SongData{
		FileName: songName,
		Data:     encodedData,
	}
	songJSON := toJSON(song)

	var uploadResponse string
	fmt.Println("Sending song to server")
	sendData(client, "uploadFile", songJSON, &uploadResponse)
	fmt.Println(uploadResponse)
}

func sendData(client clientStruct, path string, jsonString string, fillResult interface{}) {
	server := "http://" + (**client.serverURL).Host + "/"
	dataReader := strings.NewReader(jsonString)

	request, err := http.NewRequest("POST", server+path, dataReader)
	panicIf(err)
	request.Header.Set("Content-Type", "application/json")
	defer request.Body.Close()

	requestClient := &http.Client{}
	resp, err := requestClient.Do(request)
	panicIf(err)
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(fillResult)
	panicIf(err)
	return
}

func queueURL(client clientStruct) {

}

func queueSearch(client clientStruct) {

}

//*******************
// Server code here
//*******************

func startServer(server serverStruct) {
	log.Println("Starting qli server")

	ipAddr, err := serverAddress()
	panicIf(err)
	log.Println("Tell your friends to send requests to " + ipAddr + ":" + strconv.Itoa(*server.port))
	hashExistingFiles()

	r := mux.NewRouter()
	r.HandleFunc("/checkHash", checkHash).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	r.HandleFunc("/uploadFile", uploadFile).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")

	r.NotFoundHandler = http.HandlerFunc(errorPage)

	portString := strconv.Itoa(*server.port)
	http.ListenAndServe(":"+portString, handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(r))
}

func hashExistingFiles() {
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		err := os.Mkdir(downloadFolder, 0777)
		panicIf(err)
		return
	}
	files, _ := ioutil.ReadDir(downloadFolder)

	totalMp3s := 0
	for _, file := range files {
		songName := file.Name()
		if songName[len(songName)-4:] == ".mp3" {
			fmt.Println("Hashing: " + songName)
			totalMp3s++
			dat, err := ioutil.ReadFile(downloadFolder + "/" + songName)
			panicIf(err)
			hash := md5.Sum(dat)
			hashHolder[string(hash[:])] = songName
		}
	}

	log.Println("Hashed existing serverSongs folder contents, which was " + strconv.Itoa(totalMp3s) + " total .mp3 files")
}

func checkHash(w http.ResponseWriter, r *http.Request) {
	sentHash, err := ioutil.ReadAll(r.Body)
	panicIf(err)

	var msg string
	_, exists := hashHolder[string(sentHash)]
	if exists {
		//TODO: queuesong
		msg = MsgDontSendFile
		w.WriteHeader(200)
	} else {
		msg = MsgSendFullFile
		w.WriteHeader(404)
	}

	log.Println(msg + " (ip: " + clientAddress(r) + ")")
	w.Write([]byte(toJSON(msg)))
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var song SongData
	err := json.NewDecoder(r.Body).Decode(&song)
	panicIf(err)
	fmt.Println(song.FileName)

	cleanFileName := sanitize.BaseName(song.FileName)
	decodedRawData, err := base64.StdEncoding.DecodeString(song.Data)
	panicIf(err)
	hash := md5.Sum(decodedRawData)
	songName, exists := hashHolder[string(hash[:])]

	if exists {
		msg := (`Recieved raw data for "` + cleanFileName + `", aka "` + songName + `". Request rejected, send hashes before full song data.`)
		w.WriteHeader(403)
		log.Println(msg + " (ip: " + clientAddress(r) + ")")
		w.Write([]byte(msg))
		return
	}
	file, err := os.Create(downloadFolder + "/" + cleanFileName + ".mp3")
	panicIf(err)
	defer file.Close()
	_, err = file.Write(decodedRawData)
	panicIf(err)
	file.Sync()

	queueSong(cleanFileName, string(hash[:]), songHolder{clientAddress(r), cleanFileName, "", ""})

	w.WriteHeader(200)
	log.Println(`Song "` + cleanFileName + `" successfully added to queue. (ip: ` + clientAddress(r) + ")")
	w.Write([]byte(toJSON(MsgSongQueued)))
}

func queueSong(name string, hash string, song songHolder) {
	hashHolder[hash] = name
	songQueue = append(songQueue, song)
}

func serverAddress() (string, error) {
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

func clientAddress(r *http.Request) string {
	reg := regexp.MustCompile(`\[.*\]`)
	ipAddr := reg.FindString(r.RemoteAddr)
	ipAddr = ipAddr[1 : len(ipAddr)-1]
	if ipAddr == "::1" {
		ipAddr = "localhost"
	}
	return ipAddr
}

func errorPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(403)
	w.Write([]byte("Page access forbidden with current credentials."))
}
