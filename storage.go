package main

import (
    "fmt"
    _"io/ioutil"
    "net/http"
)

var StorageZone = "https://la.storage.bunnycdn.com/photon/"

func UploadToCDN(file string, path string) {
    client := &http.Client{}
    
    szpath := fmt.Sprintf("%v%v", StorageZone, path)
    req, _ := http.NewRequest("PUT", szpath, nil)
    req.Header.Add("AccessKey", Config.StoragePassWrite)
    
}
