package utils

import "encoding/json"

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
