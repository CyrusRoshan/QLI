package main

import (
	"bytes"
	"crypto/md5"
	"errors"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gorilla/mux"
	"gopkg.in/alecthomas/kingpin.v2"
)

type serverStruct struct {
	port   *int
	weight *int
}

type clientStruct struct {
	serverURL  **url.URL
	songFile   **os.File
	songURL    **url.URL
	songSearch *string
}

type songHolder struct {
	selfSubmitted bool
	fileName      string
	fileURL       string
	fileSearch    string
}

var songQueue []songHolder
var hashHolder map[string]string

var downloadFolder, _ = filepath.Abs("./serverSongs")

func main() {
	app := kingpin.New("qli", "queuemmand line interface to queue music between friends")

	serverMode := app.Command("server", "Start qli in server mode.")
	server := serverStruct{
		serverMode.Arg("port", "Port to run qli server on.").Default("3005").Int(),
		serverMode.Arg("ratio", "How many songs you can play for each song any other user can play").Default("1").Int(),
	}

	clientMode := app.Command("client", "Start qli in client mode")
	client := clientStruct{
		clientMode.Arg("server", "qli server IP address and port number").Required().URL(),
		clientMode.Flag("file", "Play an mp3 file on the server.").Short('f').File(),
		clientMode.Flag("url", "Play music from a youtube or spotify url.").Short('u').URL(),
		clientMode.Flag("search", "Search for a song on spotify and select one to be played").Short('s').String(),
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
			println("Please enter a file, url, or song to search, in order to queuemmand a song")
		}
	}
}

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}

//*******************
// Client code here
//*******************

func queueFile(client clientStruct) {
	server := "http://" + (**client.serverURL).Host

	println("Reading file")
	dat, err := ioutil.ReadFile((*client.songFile).Name())
	panicIf(err)

	println("Hashing file")
	hash := md5.Sum(dat)

	println("Checking if file is stored on qli server")
	hashReader := bytes.NewReader(hash[:])
	resp, err := http.Post(server, "data", hashReader)
	panicIf(err)

	println(resp)
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

	router := mux.NewRouter()
	router.HandleFunc("/checkHash", checkHash).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")
	router.HandleFunc("/uploadFile", uploadFile).
		Methods("POST").
		HeadersRegexp("Content-Type", "application/(text|json)")

	router.NotFoundHandler = http.HandlerFunc(errorPage)

	portString := strconv.Itoa(*server.port)
	http.ListenAndServe(":"+portString, router)
}

func hashExistingFiles() {
	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		err := os.Mkdir(downloadFolder, 0777)
		panicIf(err)
		return
	}
	files, _ := ioutil.ReadDir(downloadFolder)
	for _, file := range files {
		dat, err := ioutil.ReadFile(file.Name())
		panicIf(err)
		hash := md5.Sum(dat)
		hashHolder[string(hash[:])] = file.Name()
	}

	log.Println("Hashed existing serverSongs folder contents, which was " + strconv.Itoa(len(files)) + " total songs")
}

func checkHash(w http.ResponseWriter, r *http.Request) {
	sentHash, err := ioutil.ReadAll(r.Body)
	panicIf(err)

	var msg string
	songName, exists := hashHolder[string(sentHash)]
	if exists {
		msg = ("Hash recieved for song \"" + songName + "\", no need to send full file.")
		w.WriteHeader(200)
	} else {
		msg = ("Song hash not found, send full base64 encoded file.")
		w.WriteHeader(404)
	}

	log.Println(msg + " (ip: " + clientAddress(r) + ")")
	w.Write([]byte(msg))
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Gorilla!\n"))
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
	w.Header().Set("Content-Type", "application/json") // send content in JSON
	w.Write([]byte(`{"status": "nonexistent page"}` + "\n"))
}
