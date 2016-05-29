package main

import (
	"bytes"
	"crypto/md5"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

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
		println(server.port)

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
