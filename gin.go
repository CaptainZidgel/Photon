package main

import (
	_ "bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	_ "golang.org/x/image/draw"
	"golang.org/x/text/unicode/norm"
	_ "image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	_ "mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type config struct {
	DBun                string
	DBpass              string
	DBdb                string
	StoragePassReadOnly string
	StoragePassWrite    string
	CSecret             string
	PassCost            int
}

var Config config

func AssembleDriverStr() string {
	return fmt.Sprintf("%v:%v@/%v", Config.DBun, Config.DBpass, Config.DBdb)
}

var reserved_unames_arr []string = []string{"a", "b"}

type User struct { // &user.id, &user.Username, &user.Displayname, &user.Avatar
	id          int
	Username    string
	Displayname string
	Avatar      string
	Bio         string
}

func GetUser(database *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var myUser User
		username := sessions.Default(c).Get("username")
		if username != nil {
			err := database.QueryRow("SELECT user_id, username, displayname, avatar, bio FROM users WHERE username = ?", username).Scan(&myUser.id, &myUser.Username, &myUser.Displayname, &myUser.Avatar, &myUser.Bio)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Fatal(err)
				} else {
					c.Set("myUser", nil)
				}
			}
			myUser.Avatar = StorageZoneRead + myUser.Avatar
			c.Set("myUser", myUser)
		} else {
			c.Set("myUser", nil)
		}
	}
}

func main() {
	pcost, _ := strconv.Atoi(os.Getenv("PHOTON_PASSWORD_COST"))
	Config = config{
		DBun:                os.Getenv("PHOTON_DB_USERNAME"),
		DBpass:              os.Getenv("PHOTON_DB_PASSWORD"),
		DBdb:                os.Getenv("PHOTON_DB"),
		StoragePassReadOnly: os.Getenv("PHOTON_CDN_READ_ACCESS_KEY"),
		StoragePassWrite:    os.Getenv("PHOTON_CDN_WRITE_ACCESS_KEY"),
		CSecret:             os.Getenv("PHOTON_COOKIE_SECRET"),
		PassCost:            pcost, //bcrypt
	}

	reserved_unames := ReservedNames("reserved_names.csv")

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Printf("conf: %v\n", Config)
	nfkd := norm.NFKD.String

	db, err := sql.Open("mysql", AssembleDriverStr())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	//PurgeLostMedia(db)

	/* Prepared statements need only be used for queries you anticipate will be frequent.								*/
	sqlINSERTuser, err := db.Prepare("INSERT INTO users(username, displayname, pass, avatar, bio) VALUES(?, ?, ?, '', '')")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlINSERTuser.Close()

	sqlSELECTuserID, err := db.Prepare("SELECT user_id, username, displayname, avatar, bio FROM users WHERE user_id = ?") //You can only placeholder for VALUES(?) and WHERE thing = ?. Thing CANNOT be a placeholder. CRINGE!
	if err != nil {
		log.Fatal(err)
	}
	defer sqlSELECTuserID.Close()

	sqlSELECTuserNAME, err := db.Prepare("SELECT user_id, username, displayname, avatar, bio FROM users WHERE username = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlSELECTuserNAME.Close()

	sqlSELECTuserPASS, err := db.Prepare("SELECT pass FROM users WHERE username = ?") //get pass for username
	if err != nil {
		log.Fatal(err)
	}
	defer sqlSELECTuserPASS.Close()

	sqlSELECTphotos, err := db.Prepare("SELECT ref, photo_id, gallery_id, datetaken, fstop, iso, model, lens FROM photos WHERE gallery_id = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlSELECTphotos.Close()

	sqlSELECTgals, err := db.Prepare("SELECT gallery_id, thumb, description, uploaded FROM galleries WHERE owner_id = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlSELECTgals.Close()

	sqlDELETEgals, err := db.Prepare("DELETE FROM galleries WHERE gallery_id = ? AND owner_id = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDELETEgals.Close()

	sqlUPDATEuserPASS, err := db.Prepare("UPDATE users SET pass = ? WHERE username = ?") //replace the password hash for this username : sqlUPDATEuserPASS.Exec(newhash, username)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlUPDATEuserPASS.Close()

	sqlUPDATEuserAVATAR, err := db.Prepare("UPDATE users SET avatar = ? WHERE username = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlUPDATEuserAVATAR.Close()

	sqlUPDATEuserBIO, err := db.Prepare("UPDATE users SET bio = ? WHERE username = ?")
	if err != nil {
		log.Fatal(err)
	}
	defer sqlUPDATEuserBIO.Close()

	/*																																																		*/

	rout := gin.Default()
	//rout.LoadHTMLGlob("views/*")
	rout.HTMLRender = loadTemplates("./views")

	store := cookie.NewStore([]byte(Config.CSecret))
	rout.Use(sessions.Sessions("session", store))

	rout.Use(GetUser(db))

	rout.GET("/", func(c *gin.Context) {
		myUser, _ := c.Get("myUser")
		c.HTML(http.StatusOK, "index.tmpl", gin.H{"Nums": []int{1, 2, 3, 5}, "myUser": myUser})
	})

	rout.GET("/register", func(c *gin.Context) {
		c.HTML(http.StatusOK, "register.tmpl", gin.H{"registration": true})
	})
	rout.POST("/register", func(c *gin.Context) {
		username := c.PostForm("username")
		if utf8.ValidString(username) {
			cleanUN := nfkd(username)

			if reserved_unames[username] || !VerifyUsername(username) {
				c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"error": "This username is not permitted", "registration": true})
				return
			}

			var user User
			notok := sqlSELECTuserNAME.QueryRow(cleanUN).Scan(&user.id, &user.Username, &user.Displayname, &user.Avatar, &user.Bio)
			if notok != sql.ErrNoRows {
				if notok != nil {
					log.Fatal(notok)
				}
				c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"error": "This username is already taken", "registration": true})
				return
			} else { //error IS "no rows found"
				display_name := c.DefaultPostForm("display", username)
				username = cleanUN

				err := VerifyPasswordBasic(true, c.PostForm("password"), c.PostForm("conf_password"), "")
				if err != nil {
					c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"error": err.Error(), "username": username, "registration": true})
					return
				}
				passw := []byte(c.PostForm("password"))
				//generate password hash
				hash, err := bcrypt.GenerateFromPassword(passw, Config.PassCost)
				if err != nil {
					log.Panic(err)
				}

				//create user in db
				_, err2 := sqlINSERTuser.Exec(username, display_name, hash) //Exec does not return useful information related to the results of the query, therefore it is only appropriate for INSERT & UPDATE statements.
				if err2 != nil {
					log.Panic(err2)
				}

				c.Redirect(http.StatusSeeOther, "/")
				c.Abort()
			}
		} else {
			log.Fatal("Not valid string for username", username)
		}
	})

	rout.GET("/login", func(c *gin.Context) {
		myUser, _ := c.Get("myUser")
		c.HTML(http.StatusOK, "register.tmpl", gin.H{"myUser": myUser})
	})
	rout.POST("/login", func(c *gin.Context) {
		session := sessions.Default(c)

		un := c.PostForm("username")
		p := c.PostForm("password")

		//var user User
		var tempPass string
		sqlSELECTuserPASS.QueryRow(un).Scan(&tempPass)
		err := bcrypt.CompareHashAndPassword([]byte(tempPass), []byte(p))
		var errtxt string
		if err != nil {
			if errors.Is(err, bcrypt.ErrHashTooShort) {
				errtxt = "Server failed authentication, try again later"
				log.Println("WARNING - Password authentication attempted to check an invalid hash")
			} else {
				errtxt = "Authentication failed (username or password not recognized)"
			}
			c.HTML(401, "register.tmpl", gin.H{"error": errtxt})
			return
		}
		session.Set("username", un)
		session.Save()
		c.String(200, "GOOD JOB SIR WELCOME")
	})

	rout.GET("/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", nil)
		session.Save()

		c.Redirect(302, "/")
		c.Abort()
	})

	rout.GET("/update_password", func(c *gin.Context) {
		//CHECK HERE IF LOGGED IN
		c.HTML(http.StatusOK, "update_password.tmpl", gin.H{})
	})
	rout.POST("/update_password", func(c *gin.Context) {
		myUser, _ := c.Get("myUser")

		if myUser == nil {
			c.String(http.StatusUnauthorized, "You must be logged in.")
			return
		} else {
			un := myUser.(User).Username

			oldpass := c.PostForm("oldpassword")
			newpass := c.PostForm("newpassword") //new desired password
			conf_new := c.PostForm("conf_newpassword")
			err := VerifyPasswordBasic(false, newpass, conf_new, oldpass)
			if err != nil {
				c.String(http.StatusForbidden, err.Error())
				//c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"Error": err.Error(), "username": username})
				return
			}

			//confirm user knows old pass
			var tempPass string
			sqlSELECTuserPASS.QueryRow(un).Scan(&tempPass)
			err = bcrypt.CompareHashAndPassword([]byte(tempPass), []byte(oldpass))
			if err != nil {
				//c.Redirect(401, "BAD PASSWORD")	//THIS NEEDS TO BE FIXEd
				c.String(http.StatusForbidden, "Old password is incorrect") //THIS DOESNT STOP THIS THREAD
				return
			}

			//create new pass hash
			hash, err := bcrypt.GenerateFromPassword([]byte(newpass), Config.PassCost)
			if err != nil {
				log.Panic(err)
				c.String(500, err.Error())
				return
			}

			//
			//update password
			_, err = sqlUPDATEuserPASS.Exec(hash, un)
			if err != nil {
				log.Panic(err)
				c.String(500, err.Error())
				return
			}
			c.String(http.StatusOK, "Password updated")
			return
		}

	})

	rout.GET("/upload", func(c *gin.Context) {
		//if registered
		myUserI, _ := c.Get("myUser") //this second result is a bool if the key exists, but it always exists: its just nil if you're not logged in

		if myUserI != nil {
			myUser := myUserI.(User)
			c.HTML(http.StatusOK, "upload_form.tmpl", gin.H{"myUser": myUser})
		} else {
			//c.String(http.StatusUnauthorized, "You must be logged in to upload.")
			c.HTML(http.StatusUnauthorized, "upload_form.tmpl", gin.H{"Error": "You must be logged in to upload"})
			return
		}
	})
	rout.POST("/upload", func(c *gin.Context) {
		myUserI, _ := c.Get("myUser")
		var myUser User

		if myUserI == nil {
			c.HTML(http.StatusUnauthorized, "index.tmpl", gin.H{"Error": "You must be logged in to upload"})
			return
		}

		myUser = myUserI.(User)

		gal_descrip := c.PostForm("desc")

		uploaded_gallery, ug_err := c.MultipartForm()
		if ug_err != nil {
			c.String(http.StatusBadRequest, fmt.Sprintf("Form err: %s", ug_err.Error()))
			return
		}
		photos := uploaded_gallery.File["files"] //get the parameter named "files" from the form
		gal := NewGallery(db, myUser.id, NowDateString(), gal_descrip)
		gid := int(gal.Id)
		for p_index, photoheader := range photos {
			file, err := photoheader.Open() //get associated file for parameter (type: File)
			if err != nil {
				c.String(500, "Error uploading file: "+err.Error())
				c.Abort()
			}
			defer file.Close()

			buff := make([]byte, 512) //verify image is valid https://stackoverflow.com/a/38175140/12514997
			n_read, err := file.Read(buff)
			if n_read < 1 {
				c.String(500, "End of File reached")
				c.Abort()
			}
			if err != nil {
				c.String(500, "Error reading file for validation: "+err.Error())
				c.Abort()
			}
			file.Seek(0, 0) //file.Read(buff) consumed our bytes, reset to start

			var content_type string = http.DetectContentType(buff)
			if strings.HasPrefix(content_type, "image/") {
				var extension string = strings.TrimPrefix(content_type, "image/")
				fmt.Println("ext", extension)

				var scrubbed_image []byte = EraseGPS(file)
				var thumb []byte

				var main_path, thumb_path string = DefinePath(myUser.Username, scrubbed_image, extension, "image")
				err := UploadToCDN(bytes.NewReader(scrubbed_image), main_path)
				if err != nil {
					panic(err)
				}
				if p_index == 0 { //if this is the first image in the set
					var buf bytes.Buffer
					file.Seek(0, 0)
					buf.ReadFrom(file)
					thumb = CreateThumb(buf.Bytes(), extension, false)
					err := UploadToCDN(bytes.NewReader(thumb), thumb_path)
					if err != nil {
						panic(err)
					}
				}

				if gal.Thumb == "" {
					gal.Thumb = thumb_path
					UpdateGalleryDB(db, gal)
					fmt.Println("Setting thumb", thumb_path)
				}

				exif, err := ParseExif(bytes.NewReader(scrubbed_image))
				if err != nil {
					if err.Error() == "no exif data" {
						exif = make(Exif)
						exif["Date Taken"], exif["F-Stop"], exif["ISO"], exif["Model"], exif["Lens"] = "", "", "", "", ""
					} else {
						panic(err)
					}
				}
				_ = NewPhoto(db, main_path, gid, exif) //created a new photo and inserted it into the DB

				/*
				   thumb := Scale(uploaded, image.Rect(0, 0, 200, 200), draw.ApproxBiLinear)
				   f, _ := os.Create("testrescale.jpg")
				   defer f.Close()
				   jpeg.Encode(f, thumb, nil)
				*/
			} else {
				c.String(415, "Unsupported file type") //415 -> Media type unsupported
				c.Abort()
			}
		}

		c.String(200, "OK")
	})

	rout.GET("/p/:path", func(c *gin.Context) {
		myUser, _ := c.Get("myUser")

		path := c.Param("path")
		var user User
		err := sqlSELECTuserNAME.QueryRow(path).Scan(&user.id, &user.Username, &user.Displayname, &user.Avatar, &user.Bio)
		user.Avatar = StorageZoneRead + user.Avatar
		if err != nil {
			if err == sql.ErrNoRows {
				c.String(404, "This profile doesn't exist")
				c.Abort()
				return
			} else {
				log.Fatal(err)
			}
		}

		rows, err := sqlSELECTgals.Query(user.id)
		defer rows.Close()
		gals := make([]Gallery, 0)
		for rows.Next() {
			var gallery Gallery
			if err := rows.Scan(&gallery.Id, &gallery.Thumb, &gallery.Description, &gallery.Uploaded); err != nil {
				panic(err)
			}
			PopulateGallery(sqlSELECTphotos, &gallery)
			gals = append(gals, gallery)
		}
		bytes, err := json.Marshal(gals)
		if err != nil {
			panic(err)
		}
		jsonGals := string(bytes[:])

		var SameUser bool
		if myUser != nil && myUser.(User).id == user.id {
			SameUser = true
		} else {
			SameUser = false
		}
		fmt.Println("Same user:", SameUser)
		c.HTML(200, "profile.tmpl", gin.H{"User": user, "Galleries": gals, "myUser": myUser, "SameUser": SameUser, "jsonGals": jsonGals})
	})

	rout.GET("/other/:o", func(c *gin.Context) {
		o := c.Param("o")
		c.String(200, nfkd(o))
	})

	rout.POST("/delete_gal", func(c *gin.Context) {
		myUserI, _ := c.Get("myUser")
		if myUserI == nil {
			c.HTML(http.StatusUnauthorized, "index.tmpl", gin.H{"Error": "You must be logged in to delete this gallery"})
			return
		}
		var myUser User = myUserI.(User)

		gid := c.PostForm("gallery-id")
		log.Printf("Received gallery deletion request, ID %v\n", gid)
		res, err := sqlDELETEgals.Exec(gid, myUser.id) //res has methods LastInsertId() or RowsAffected().
		if err != nil {
			panic(err)
			c.String(500, "Error deleting gal")
			c.Abort()
			return
		}
		rows, _ := res.RowsAffected()
		if rows != 1 {
			fmt.Println("BRUH!!!", rows)
			c.String(500, "Error deleting gal2")
			c.Abort()
			return
		}
		c.String(200, "OK")
		return
	})

	rout.POST("/update_avatar", func(c *gin.Context) {
		myUserI, _ := c.Get("myUser")
		if myUserI == nil {
			c.HTML(http.StatusUnauthorized, "index.tmpl", gin.H{"Error": "You must be logged in to change your profile picture"})
			return
		}

		var myUser User = myUserI.(User)

		fileheader, _ := c.FormFile("pfp")
		file, err := fileheader.Open()
		if err != nil {
			c.String(500, "Error uploading file: "+err.Error())
			return
		}
		defer file.Close()

		vbuf := make([]byte, 512) //verify image is valid https://stackoverflow.com/a/38175140/12514997
		n_read, err := file.Read(vbuf)
		if n_read < 1 {
			c.String(500, "End of File reached")
			c.Abort()
		}
		if err != nil {
			c.String(500, "Error reading file for validation: "+err.Error())
			c.Abort()
		}
		file.Seek(0, 0) //file.Read(buff) consumed our bytes, reset to start

		var content_type string = http.DetectContentType(vbuf)
		if strings.HasPrefix(content_type, "image/") {
			var extension string = strings.TrimPrefix(content_type, "image/")

			var scrubbed_image []byte = EraseGPS(file)
			var thumb []byte

			var main_path, _ string = DefinePath("Noneedforthisparam", scrubbed_image, extension, "avatar")
			thumb = CreateThumb(scrubbed_image, extension, true)
			err := UploadToCDN(bytes.NewReader(thumb), main_path)
			if err != nil {
				panic(err)
			}
			_, err = sqlUPDATEuserAVATAR.Exec(main_path, myUser.Username)
			if err != nil {
				panic(err)
			}

			c.JSON(201, gin.H{"url": StorageZoneRead + main_path})
			return
		}
		c.String(415, "Unsupported file type")
	})

	rout.POST("/update_bio", func(c *gin.Context) {
		myUserI, _ := c.Get("myUser")
		if myUserI == nil {
			c.HTML(http.StatusUnauthorized, "index.tmpl", gin.H{"Error": "You must be logged in to change your profile picture"})
			return
		}
		myUser := myUserI.(User)

		bio := c.PostForm("biography")
		//no validation lmao
		_, err = sqlUPDATEuserBIO.Exec(bio, myUser.Username)
		if err != nil {
			panic(err)
		}

		c.JSON(200, gin.H{})
		return
	})

	fmt.Println("Serving on 0.0.0.0:8080")
	rout.Run()
}

func loadTemplates(dir string) multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	layouts, err := filepath.Glob(dir + "/layouts/*") //Glob returns the names of all files matching pattern
	if err != nil {
		log.Fatal(err)
	}

	includes, err := filepath.Glob(dir + "/templates/*")
	if err != nil {
		log.Fatal(err)
	}

	for _, include := range includes {
		layoutCopy := make([]string, len(layouts))
		copy(layoutCopy, layouts)
		files := append(layoutCopy, include)
		r.AddFromFiles(filepath.Base(include), files...)
	}
	return r
}
