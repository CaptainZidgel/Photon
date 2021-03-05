package main

import (
    "fmt"
    _"io/ioutil"
    "net/http"
    "io"
    "crypto/md5"
    "time"
    "errors"
)

var StorageZoneRead = "https://photon.b-cdn.net/" 
var StorageZoneWrite = "https://la.storage.bunnycdn.com/photon/"

func UploadToCDN(file io.Reader, path string) (error) {
    client := &http.Client{}
    
    szpath := fmt.Sprintf("%v%v", StorageZoneWrite, path) //append the path to the StorageZoneWrite root
    req, err := http.NewRequest("PUT", szpath, file)
    if err != nil {
        panic(err)
    }
    req.Header.Add("AccessKey", Config.StoragePassWrite)
    
    response, err := client.Do(req)
    if err != nil {
        panic(err)
        return err
    }
    defer response.Body.Close()
    //probably should read the status code here!      
    fmt.Println(response.StatusCode)
    if response.StatusCode != 201 {
        return errors.New("Undesired status code during upload: "+response.Status)
    }
    return nil
}

//Should create/import some sort of algo to give every image a hash (not necessarily based on image itself) for cdn storage. +_thumb where appropriate.
func DefinePath(username string, file []byte, ext string) (string, string) {
    image_hash := md5.Sum(file)
    t := time.Now()
    year := t.Year()
    yearday := t.YearDay()
    
    final := fmt.Sprintf("%s/%d/%d/%x%s.%s", username, year, yearday, image_hash, "", ext)
    thumb_final := fmt.Sprintf("%s/%d/%d/%x%s.%s", username, year, yearday, image_hash, "_thumb", ext)
    return final, thumb_final
}
