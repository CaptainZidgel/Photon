package main

import (
    "fmt"
    _"io/ioutil"
    "net/http"
    "io"
)

var StorageZone = "https://la.storage.bunnycdn.com/photon/"

func UploadToCDN(file io.Reader, path string) {
    client := &http.Client{}
    
    szpath := fmt.Sprintf("%v%v", StorageZone, path) //append the path to the StorageZone root
    req, _ := http.NewRequest("PUT", szpath, file)
    req.Header.Add("AccessKey", Config.StoragePassWrite)
    
    response, err := client.Do(req)
    if err != nil {
        panic(err)
        return
    }
    defer response.Body.Close()
    //probably should read the status code here!      
    fmt.Println(response.Status)
}

//Should create/import some sort of algo to give every image a hash (not necessarily based on image itself) for cdn storage. +_thumb where appropriate.
func DefinePath() string {
    return ""
}
