package utils

import (
	"encoding/json"
	"os"
)

func PanicIf(err error) {
	if err != nil {
		panic(err)
	}
}

func ToJSON(data interface{}) string {
	jsonString, err := json.Marshal(data)
	PanicIf(err)
	return string(jsonString)
}

func MakeDirIfNotExist(folder string) {
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		err := os.Mkdir(folder, 0777)
		PanicIf(err)
		return
	}
}
