package client

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyrusroshan/qli/server"
	"github.com/cyrusroshan/qli/utils"
)

type ClientStruct struct {
	ServerURL  **url.URL
	SongFile   **os.File
	SongURL    **url.URL
	SongSearch *string
}

func QueueFile(client ClientStruct) {
	songPath := (*client.SongFile).Name()
	songName := filepath.Base((*client.SongFile).Name())
	dat, err := ioutil.ReadFile(songPath)
	utils.PanicIf(err)

	hash := md5.Sum(dat)

	var hashResponse string
	SendData(client, "checkHash", string(hash[:]), &hashResponse)

	if hashResponse != server.MsgSendFullFile {
		fmt.Println(hashResponse)
		return
	}

	encodedData := base64.StdEncoding.EncodeToString(dat)
	song := server.SongData{
		FileName: songName,
		Data:     encodedData,
	}
	songJSON := utils.ToJSON(song)

	var uploadResponse string
	fmt.Println("Sending song to server")
	SendData(client, "uploadFile", songJSON, &uploadResponse)
	fmt.Println(uploadResponse)
}

func SendData(client ClientStruct, path string, jsonString string, fillResult interface{}) {
	server := "http://" + (**client.ServerURL).Host + "/"
	dataReader := strings.NewReader(jsonString)

	request, err := http.NewRequest("POST", server+path, dataReader)
	utils.PanicIf(err)
	request.Header.Set("Content-Type", "application/json")
	defer request.Body.Close()

	requestClient := &http.Client{}
	resp, err := requestClient.Do(request)
	utils.PanicIf(err)
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(fillResult)
	utils.PanicIf(err)
	return
}

func QueueURL(client ClientStruct) {

}

func QueueSearch(client ClientStruct) {

}