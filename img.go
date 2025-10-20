package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/dsoprea/go-exif/v3"
	_ "github.com/go-sql-driver/mysql"
	"github.com/kolesa-team/goexiv"
	"golang.org/x/image/draw"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	_ "io/ioutil"
	"log"
	"math"
	_ "os"
	"strconv"
	"strings"
	_ "time"
)

var maxWidth int = 200
var maxHeight int = 200

type Exif map[string]string

// Set, Lua style
var ExifTags = map[string]string{
	"DateTimeOriginal": "Date Taken",
	"LensModel":        "Lens",
	"Model":            "Model",
	"ISOSpeedRatings":  "ISO",
	"FNumber":          "F-Stop", //this is a string of 'x/y', needs to be  F{x div y}
}

// Take a file already opened, return the exif dictionary
func ParseExif(file io.Reader) (output Exif, err error) {
	result := make(Exif)

	bytes, err := exif.SearchAndExtractExifWithReader(file)
	if err != nil {
		if err.Error() == "no exif data" { //I couldn't compare err and exif.ErrNoExif for some reason
			return nil, err //uninit, nil map
		} else {
			panic(err)
		}
	}

	fmt.Println("Found exif... searching...")
	opt := exif.ScanOptions{} //I'm basically copying this from https://github.com/photoprism/photoprism/blob/develop/internal/meta/exif.go
	entries, _, err := exif.GetFlatExifData(bytes, &opt)
	for _, entry := range entries {
		if entry.TagName != "" && entry.Formatted != "" {
			result[entry.TagName] = strings.Split(entry.FormattedFirst, "\x00")[0] //I don't understand what this formatting does but nice
		}
	}

	final := make(Exif)
	for key, newkey := range ExifTags {
		if value, exists := result[key]; exists {
			final[newkey] = value
		} else {
			final[newkey] = ""
		}
	}

	return final, nil
}

func EraseGPS(file io.Reader) []byte {
	//reader to bytes
	var buf bytes.Buffer
	n_read, er := buf.ReadFrom(file)
	if er != nil {
		panic(er)
	}
	if n_read < 1 {
		log.Println("UHHH LESS THAN 1 BYTE READ")
	}

	img, err := goexiv.OpenBytes(buf.Bytes())
	if err != nil {
		panic(err)
	}
	err = img.ReadMetadata()
	if err != nil {
		panic(err)
	}

	//heq if i know why but without this line, trying to set exifstrings will cause a segfault when you try to read the image later :DDD
	_ = img.GetExifData().AllTags()

	//img.SetExifString("Exif.GPSInfo.0x001f", "")
	img.SetExifString("Exif.GPSInfo.GPSLatitudeRef", "")
	img.SetExifString("Exif.GPSInfo.GPSLatitude", "")
	img.SetExifString("Exif.GPSInfo.GPSLongitude", "")
	img.SetExifString("Exif.GPSInfo.GPSLongitudeRef", "")
	img.SetExifString("Exif.GPSInfo.Altitude", "")
	img.SetExifString("Exif.GPSInfo.DestBearing", "")
	img.SetExifString("Exif.GPSInfo.Speed", "")
	img.SetExifString("Exif.GPSInfo.ImgDirection", "")

	//return SCRUBBED!!
	//return img.GetBytes() OR change to reader?
	return img.GetBytes()
}

func ExifFromDB(db *sql.DB, id int) Exif {
	var exif Exif
	row := db.QueryRow("SELECT datetaken, fstop, iso, model, lens FROM photos WHERE user_id = ?", id)
	err := row.Scan(exif["Date Taken"], exif["F-Stop"], exif["ISO"], exif["Model"], exif["Lens"])
	if err != nil {
		panic(err)
	}
	return exif
}

func ExifFromStruct(p Photo) Exif {
	exif := make(Exif)
	exif["Date Taken"] = p.Datetaken
	exif["F-Stop"] = p.Fstop
	exif["ISO"] = string(p.ISO)
	exif["Model"] = p.Model
	exif["Lens"] = p.Lens
	return exif
}

type Photo struct {
	Id         int64
	Reference  string
	Gallery_id int

	Datetaken string
	Fstop     string
	ISO       int
	Model     string
	Lens      string

	Exif Exif
}

type Gallery struct {
	Id          int64
	Owner       int
	Thumb       string
	Description string
	Uploaded    string
	Photos      []Photo
}

func InsertPhotoIntoDatabase(db *sql.DB, photo Photo) int64 {
	r, e := db.Exec("INSERT INTO photos(ref, gallery_id, datetaken, fstop, iso, model, lens) VALUES(?, ?, ?, ?, ?, ?, ?)",
		photo.Reference,
		photo.Gallery_id,
		/*photo.Exif["Date Taken"],
		photo.Exif["F-Stop"],
		photo.Exif["ISO"],
		photo.Exif["Model"],
		photo.Exif["Lens"],*/
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
	gallery.Thumb = ""
	gallery.Description = desc
	gallery.Uploaded = date

	//insert
	r, e := db.Exec("INSERT INTO galleries(owner_id, thumb, description, uploaded) VALUES(?, ?, ?, ?)",
		gallery.Owner,
		gallery.Thumb,
		gallery.Description,
		gallery.Uploaded,
	)
	if e != nil {
		panic(e)
	}
	lastinsert, err := r.LastInsertId()
	if err != nil {
		panic(err)
	} else {
		gallery.Id = lastinsert
	}

	return &gallery
}

func UpdateGalleryDB(db *sql.DB, gallery *Gallery) {
	_, e := db.Exec("UPDATE galleries SET owner_id = ?, thumb = ?, description = ?, uploaded = ? WHERE gallery_id = ?",
		gallery.Owner,
		gallery.Thumb,
		gallery.Description,
		gallery.Uploaded,
		gallery.Id,
	)
	if e != nil {
		panic(e)
	}
}

// the exif map is all string, but my Photo struct/SQL table is typed. Probably because I have no foresight. Nonetheless!
func NewPhoto(db *sql.DB, reference string, gallery int, exifmap Exif) *Photo {
	var ISO int
	if exifmap["ISO"] == "" {
		ISO = 0
	} else {
		ISOtemp, _ := strconv.Atoi(exifmap["ISO"])
		ISO = ISOtemp
	}
	/*
		var Fstop float64
		if exifmap["F-Stop"] == "" {
			Fstop = -0.0
		} else {
			Fn := strings.Split(exifmap["FNumber"], "/")
			fmt.Println(Fn, exifmap["FNumber"])
			Fn1, _ := strconv.Atoi(Fn[0])
			Fn2, _ := strconv.Atoi(Fn[1])
			Fstop = 	float64(Fn1) / float64(Fn2)
		}
	*/

	//datetaken := exifmap["Date Taken"]

	photo := Photo{-102, reference, gallery, exifmap["Date Taken"], exifmap["F-Stop"], ISO, exifmap["Model"], exifmap["Lens"], exifmap}
	id := InsertPhotoIntoDatabase(db, photo)
	photo.Id = id
	return &photo
}

func PopulateGallery(stmt *sql.Stmt, gallery *Gallery) { //("SELECT * FROM photos WHERE gallery_id = ?")
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
		if !strings.HasPrefix(photo.Reference, "http") {
			photo.Reference = StorageZoneRead + photo.Reference //not efficient at large scale but i dont think this is large scale to care about that?
		}
		photo.Exif = ExifFromStruct(photo)
		photos = append(photos, photo)
	}
	gallery.Photos = photos

	if len(photos) > 0 {
		if gallery.Thumb == "" {
			gallery.Thumb = photos[0].Reference
		} else {
			if !strings.HasPrefix(gallery.Thumb, "http") {
				gallery.Thumb = StorageZoneRead + gallery.Thumb
			}
		}
	}

}

// https://github.com/nfnt/resize/issues/63#issuecomment-540704731
// scaled := Scale(src, image.Rect(0, 0, 200, 200), draw.ApproxBiLinear)
func Scale(src image.Image, rect image.Rectangle, scale draw.Scaler) image.Image {
	dst := image.NewRGBA(rect)
	scale.Scale(dst, rect, src, src.Bounds(), draw.Over, nil)
	return dst
}

func DetermineNewSize(w int, h int) (int, int) { //https://stackoverflow.com/a/14731922/12514997
	var ratio float64 = math.Min(float64(maxWidth)/float64(w), float64(maxHeight)/float64(h))
	return int(float64(w) * ratio), int(float64(h) * ratio)
}

// pass a byte slice of an original image, pass the extension (so we know how to encode it), pass a bool indicating if it is an image to be automatically resized (thumbnail for uploaded image) or a profile picture with a fixed final size
func CreateThumb(original []byte, extension string, autores bool) []byte {
	rdr := bytes.NewReader(original)
	uploaded, _, err := image.Decode(rdr) //uploaded is image.Image
	if err != nil || uploaded == nil {
		panic(err)
	}

	var newW, newH int
	if autores { //if should be automatically resized
		newW, newH = DetermineNewSize(uploaded.Bounds().Dx(), uploaded.Bounds().Dy())
	} else { //this is a pfp
		newW, newH = 256, 256
	}
	thumb := Scale(uploaded, image.Rect(0, 0, newW, newH), draw.ApproxBiLinear)

	newimg := new(bytes.Buffer)
	if extension == "jpeg" || extension == "jpg" {
		err := jpeg.Encode(newimg, thumb, nil)
		if err != nil {
			panic(err)
		}
	} else if extension == "png" {
		err := png.Encode(newimg, thumb)
		if err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Sprintf("HMMMM %s", extension))
	}

	return newimg.Bytes()
}

/* I used this to fuzz my db with galleries and images

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
