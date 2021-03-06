package main

import (
    "fmt"
    "io/ioutil"
    "net/http"
    "io"
    "crypto/md5"
    "time"
    "errors"
    "strings"
    
    "database/sql"
	_ "github.com/go-sql-driver/mysql"
)

var StorageZoneRead = "https://photon.b-cdn.net/" 
var StorageZoneWrite = "https://la.storage.bunnycdn.com/photon/"

func CDNRequest(method string, path string, bodyany interface{}, akey string) (interface{}, int, error) {
    client := &http.Client{}
    
    var body io.Reader
    if bodyany != nil {
        body = bodyany.(io.Reader)
    } else {
        body = nil
    }
    
    req, err := http.NewRequest(method, path, body)
    if err != nil {
        return nil, 0, err
    }
    req.Header.Add("AccessKey", akey)
    
    response, err := client.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer response.Body.Close()
    resp_body, _ := ioutil.ReadAll(response.Body)
    
    return resp_body, response.StatusCode, nil
}

func readFromStorage(path string) (interface{}, int) {
    body, status, err := CDNRequest("GET", path, nil, Config.StoragePassReadOnly)
    if err != nil {
        panic(err)
        return nil, 0
    }
    return body, status
}

func UploadToCDN(file io.Reader, path string) (error) {
    szpath := fmt.Sprintf("%v%v", StorageZoneWrite, path) //append the path to the StorageZoneWrite root
    _, status, err := CDNRequest("PUT", szpath, file, Config.StoragePassWrite)
    if err != nil {
        panic(err)
        return err
    }

    if status != 201 {
        return errors.New("Undesired status code during upload: "+string(status))
    }
    
    //_, status, err :=
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

func PurgeLostMedia(db *sql.DB) {
    rows, err := db.Query("SELECT ref, id FROM photos")
    if err != nil {
        panic(err)
    }
    defer rows.Close()
    photos := make([]Photo, 0)
    for rows.Next() {
	    var photo Photo
	    if err := rows.Scan(&photo.Reference, &photo.Id); err != nil {
		    panic(err)
	    }
	    photos = append(photos, photo)
	}
	
	for _, photo := range photos {
	    if !strings.HasPrefix(photo.Reference, "http") {
	        szpath := fmt.Sprintf("%v%v", StorageZoneWrite, photo.Reference)
	        _, status := readFromStorage(szpath)
	        fmt.Println(status)
	        if status == 404 {
	            db.Exec("DELETE FROM photos WHERE id = ?", photo.Id)
	        }
	    }
	}
}
