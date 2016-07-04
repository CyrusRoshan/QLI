package main

import (
	"fmt"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/cyrusroshan/qli/client"
	"github.com/cyrusroshan/qli/server"
)

func main() {
	app := kingpin.New("qli", "queuemmand line interface to queue music between friends")

	serverMode := app.Command("server", "Start qli in server mode.")
	serverSettings := server.ServerStruct{
		Port: serverMode.Arg("port", "Port to run qli server on.").Default("3005").Int(),
	}

	clientMode := app.Command("client", "Start qli in client mode")
	clientSettings := client.ClientStruct{
		ServerURL:  clientMode.Arg("server", "qli server IP address and port number.").Required().URL(),
		SongFile:   clientMode.Flag("file", "Play an mp3 file on the server.").Short('f').File(),
		SongURL:    clientMode.Flag("url", "Play music from a youtube or spotify url.").Short('u').String(),
		SongSearch: clientMode.Flag("search", "Search for a song on spotify and select one to be played.").Short('s').String(),
	}

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	// Register user
	case serverMode.FullCommand():
		server.StartServer(serverSettings)

	case clientMode.FullCommand():
		if *clientSettings.SongFile != nil {
			client.QueueFile(clientSettings)
		} else if *clientSettings.SongURL != "" {
			client.QueueURL(clientSettings)
		} else if *clientSettings.SongSearch != "" {
			client.QueueSearch(clientSettings)
		} else {
			fmt.Println("Please enter a file, url, or song to search, in order to queuemmand a song")
		}
	}
}
