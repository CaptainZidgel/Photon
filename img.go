package main

import (
	"github.com/dsoprea/go-exif/v3"
	"strings"
	"io/ioutil"
	_"os"
	"fmt"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"strconv"
	_"time"
)

//Set, Lua style
var ExifTags = map[string]bool{
	"DateTimeOriginal": true,
	"LensModel": true,
	"Model": true,
	"ISOSpeedRatings": true,
	"FNumber": true,	//this is a string of 'x/y', needs to be  F{x div y}	
}

func ParseExif(file string) (output map[string]string, err error) {
	result := make(map[string]string)

	bytes, err := exif.SearchFileAndExtractExif(file) //exif.SearchAndExtractExifWithReader(*os.File)
	if err != nil { 
		if err.Error() == "no exif data" {	//I couldn't compare err and exif.ErrNoExif for some reason
			return nil, err//uninit, nil map
		} else {
			panic(err)
		} 
	}
	fmt.Println("Found exif... searching...")
	opt := exif.ScanOptions{}	//I'm basically copying this from https://github.com/photoprism/photoprism/blob/develop/internal/meta/exif.go
	entries, _, err := exif.GetFlatExifData(bytes, &opt)
	for _, entry := range entries {
		if entry.TagName != "" && entry.Formatted != "" {
			result[entry.TagName] = strings.Split(entry.FormattedFirst, "\x00")[0]	//I don't understand what this formatting does but nice
		}
	}

	final := make(map[string]string)
	for key, _ := range ExifTags {
		if value, exists := result[key]; exists {
			final[key] = value
		} else {
			final[key] = ""
		}
	}

	return final, nil
}

//unexported, i use this for fuzzing
func loadBlob(dir string) {
	items, _ := ioutil.ReadDir(dir)
    for _, item := range items {
			if item.IsDir() {continue}
			//f, _ := os.Open(item.Name())
			fmt.Println("<>><><><><><><><><><>===========Parsing", item.Name())
			dat, err := ParseExif(dir+"/"+item.Name())
			if err == nil {
				for k, v := range dat {
					fmt.Println(k, v)
				}
			}
	}
}

type Photo struct {
	Id int64
	Reference string
	Gallery_id int
	Datetaken string
	Fstop float64
	ISO int
	Model string
	Lens string
}

type Gallery struct {
	Id int64
	Owner int
	Thumb string
	Description string
	Uploaded string
	Photos []Photo
}

func InsertPhotoIntoDatabase(db *sql.DB, photo Photo) int64 {
	r, e := db.Exec("INSERT INTO photos(ref, gallery_id, datetaken, fstop, iso, model, lens) VALUES(?, ?, ?, ?, ?, ?, ?)",
					photo.Reference,
					photo.Gallery_id,
					photo.Datetaken,
					photo.Fstop,
					photo.ISO,
					photo.Model,
					photo.Lens,
				)
	if e != nil {
		fmt.Println(photo)
		panic(e)
	}
	lastinsert, err := r.LastInsertId()
	if err != nil {
		panic(e)
	} else {
		return lastinsert
	}
}

func NewGallery(db *sql.DB, owner int, date string, desc string) *Gallery {
	var gallery Gallery
	gallery.Owner = owner
	gallery.Thumb = "1234"
	gallery.Description = desc
	gallery.Uploaded = date

	//insert
	r, e := db.Exec("INSERT INTO galleries(owner_id, thumb, description, uploaded) VALUES(?, ?, ?, ?)",
					gallery.Owner,
					gallery.Thumb,
					gallery.Description,
					gallery.Uploaded,
				)
	if e != nil { panic(e) }
	lastinsert, err := r.LastInsertId()	
	if err != nil {
		panic(err)
	} else {
		gallery.Id = lastinsert
	}
	
	return &gallery
}

//the exif map is all string, but my Photo struct/SQL table is typed. Probably because I have no foresight. Nonetheless!
func NewPhoto(db *sql.DB, reference string, gallery int, exifmap map[string]string) *Photo {
	iso, _ := strconv.Atoi(exifmap["ISOSpeedRatings"])
	var Fstop float64
	if exifmap["FNumber"] == "" {
		Fstop = -0.0
	} else {
		Fn := strings.Split(exifmap["FNumber"], "/")
		fmt.Println(Fn, exifmap["FNumber"])
		Fn1, _ := strconv.Atoi(Fn[0])
		Fn2, _ := strconv.Atoi(Fn[1])
		Fstop = 	float64(Fn1) / float64(Fn2)
	}
	datetaken := exifmap["DateTimeOriginal"]

	photo := Photo{-102, reference, gallery, datetaken, Fstop, iso, exifmap["Model"], exifmap["Lens"]}
	id := InsertPhotoIntoDatabase(db, photo)
	photo.Id = id
	return &photo
}

func PopulateGallery(stmt *sql.Stmt, gallery *Gallery) {
	rows, err := stmt.Query(gallery.Id)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	
	photos := make([]Photo, 0)
	for rows.Next() {
		var photo Photo
		if err := rows.Scan(&photo.Reference, &photo.Id, &photo.Gallery_id, &photo.Datetaken, &photo.Fstop, &photo.ISO, &photo.Model, &photo.Lens); err != nil {
			panic(err)
		}
		photos = append(photos, photo)
	}
	gallery.Photos = photos
	if len(photos) > 0 {
		gallery.Thumb = photos[0].Reference
	}
}

/* I used this to fuzz my db with galleries and images
func main() {
	//loadBlob("dls")
	
	driverstr := fmt.Sprintf("%v:%v@/%v", "zidgel", password, "photon")
	db, err := sql.Open("mysql", driverstr)
	if err != nil { panic(err) }
	defer db.Close()

	i := 0
	g := int64(1)
	NewGallery(db, 5, NowDateString(), "")
	items, _ := ioutil.ReadDir("dls")
	for _, item := range items {
		if item.IsDir() {continue}
		i = i + 1
		if i % 3 == 0 {
			gal := NewGallery(db, 5, NowDateString(), "")
			g = gal.Id
			fmt.Println("New g", g)
		}
		x, err := ParseExif("dls/"+item.Name())
		if err == nil {
			p := NewPhoto(db, item.Name(), int(g), x)
			fmt.Println(p)
		} else {
			fmt.Println(err, item.Name())
		}
	}
	
}
*/
