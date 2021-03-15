package main

import (
	"log"
	"fmt"
	_ "os"
	_ "bufio"
	"bytes"
	"strings"
	_"strconv"
	"github.com/gin-gonic/gin"
	"net/http"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/text/unicode/norm"
	"unicode/utf8"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"golang.org/x/crypto/bcrypt"
	"github.com/gin-contrib/multitemplate"
	"path/filepath"
	_ "image"
	_ "image/jpeg"
	_ "image/png"
	_ "golang.org/x/image/draw"
	_ "mime/multipart"
)

func AssembleDriverStr() string {
	return fmt.Sprintf("%v:%v@/%v", Config.DBun, Config.DBpass, Config.DBdb)
}

var reserved_unames_arr []string = []string{ "a", "b" }

type User struct { // &user.id, &user.Username, &user.Displayname, &user.Avatar
	id int
	Username string
	Displayname string
	Avatar string
}

func GetUser(database *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var myUser User
		username := sessions.Default(c).Get("username")
		if username != nil {
			err := database.QueryRow("SELECT user_id, username, displayname, avatar FROM users WHERE username = ?", username).Scan(&myUser.id, &myUser.Username, &myUser.Displayname, &myUser.Avatar)
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
    fmt.Println("Ready")
    reserved_unames := ReservedNames("reserved_names.csv")

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Printf("conf: %v\n", Config)
	nfkd := norm.NFKD.String

	db, err := sql.Open("mysql", AssembleDriverStr())
	if err != nil { log.Fatal(err) }
	defer db.Close()
	
	//PurgeLostMedia(db)

	/* Prepared statements need only be used for queries you anticipate will be frequent.								*/
	sqlINSERTuser, err := db.Prepare("INSERT INTO users VALUES(?, ?, ?, ?, ?)")
	if err != nil { log.Fatal(err) }
	defer sqlINSERTuser.Close()
	
	sqlSELECTuserID, err := db.Prepare("SELECT * FROM users WHERE user_id = ?") //You can only placeholder for VALUES(?) and WHERE thing = ?. Thing CANNOT be a placeholder. CRINGE!
	if err != nil { log.Fatal(err) }
	defer sqlSELECTuserID.Close()
	
	sqlSELECTuserNAME, err := db.Prepare("SELECT user_id, username, displayname, avatar FROM users WHERE username = ?")
	if err != nil { log.Fatal(err) }
	defer sqlSELECTuserNAME.Close()
			
	sqlSELECTuserPASS, err := db.Prepare("SELECT pass FROM users WHERE username = ?") //get pass for username
	if err != nil { log.Fatal(err) }
	defer sqlSELECTuserPASS.Close()
				
	sqlSELECTphotos, err := db.Prepare("SELECT * FROM photos WHERE gallery_id = ?")
	if err != nil { log.Fatal(err) }
	defer sqlSELECTphotos.Close()
					
	sqlSELECTgals, err := db.Prepare("SELECT gallery_id, thumb, description, uploaded FROM galleries WHERE owner_id = ?")
	if err != nil { log.Fatal(err) }
	defer sqlSELECTgals.Close()
	
	sqlUPDATEuserPASS, err := db.Prepare("UPDATE users SET pass = ? WHERE username = ?") //replace the password hash for this username : sqlUPDATEuserPASS.Exec(newhash, username)
	if err != nil {log.Fatal(err) }
	defer sqlUPDATEuserPASS.Close()
	
	sqlUPDATEuserAVATAR, err := db.Prepare("UPDATE users SET avatar = ? WHERE username = ?")
	if err != nil {log.Fatal(err) }
	defer sqlUPDATEuserAVATAR.Close()
	
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
		        c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"Error": "This username is not permitted", "registration": true})
		        return
		    }
		    
			var user User
			notok := sqlSELECTuserNAME.QueryRow(cleanUN).Scan(&user.id, &user.Username, &user.Displayname, &user.Avatar)
			if notok != sql.ErrNoRows {
				if notok != nil { log.Fatal(notok) }
				c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"Error": "This username is already taken", "registration": true})
				return
			} else {	//error IS "no rows found"
				display_name := c.DefaultPostForm("display", username)
				username = cleanUN
				
				err := VerifyPasswordBasic(true, c.PostForm("password"), c.PostForm("conf_password"), "")
                if err != nil {
                    c.HTML(http.StatusForbidden, "register.tmpl", gin.H{"Error": err.Error(), "username": username})
                    return
                }
				passw := []byte(c.PostForm("password"))
				//generate password hash
				hash, err := bcrypt.GenerateFromPassword(passw, Config.PassCost)
				if err != nil { log.Panic(err) }
				
				//create user in db
				_, err2 := sqlINSERTuser.Exec(nil, username, display_name, hash, nil)	//Exec does not return useful information related to the results of the query, therefore it is only appropriate for INSERT & UPDATE statements.
				if err2 != nil { log.Panic(err2) }
				
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
		if err != nil {
			//c.Redirect(401, "BAD PASSWORD")	//THIS NEEDS TO BE FIXEd
			c.AbortWithStatus(401)			//THIS DOESNT STOP THIS THREAD
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
			    c.String(http.StatusForbidden, "Old password is incorrect")			//THIS DOESNT STOP THIS THREAD
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
	    gal := NewGallery(db, myUser.id, NowDateString(), gal_descrip)
		gid := int(gal.Id)
		/*the creation of a new gallery based on an upload, before you make sure all the photos are legit and the upload will succeed, will
		mean that canceled/errored uploads will create nonconsecutive gallery ids, but this is acceptable and even desirable*/
	
	    uploaded_gallery, ug_err := c.MultipartForm()
	    if ug_err != nil {
	        c.String(http.StatusBadRequest, fmt.Sprintf("Form err: %s", ug_err.Error()))
	        return
	    }
	    photos := uploaded_gallery.File["files"] //get the parameter named "files" from the form
	    
	    for p_index, photoheader := range photos { 
		    file, err := photoheader.Open() //get associated file for parameter (type: File)
		    if err != nil { 
			    c.String(500, "Error uploading file: " + err.Error()) 
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
			    c.String(500, "Error reading file for validation: " + err.Error()) 
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
		            thumb = CreateThumb(buf.Bytes(), extension, true)
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
		                exif["Date Taken"], exif["F-Stop"], exif["ISO"], exif["Model"], exif["Lens"] = "","","","",""
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
		err := sqlSELECTuserNAME.QueryRow(path).Scan(&user.id, &user.Username, &user.Displayname, &user.Avatar)
		user.Avatar = StorageZoneRead + user.Avatar
		if err != nil {
			if err == sql.ErrNoRows { 
				c.String(404, "This profile doesn't exist")
				c.Abort() 
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

		var SameUser bool
		if myUser != nil && myUser.(User).id == user.id {
			SameUser = true
		} else {
			SameUser = false
		}
		fmt.Println("Same user:", SameUser)
		c.HTML(200, "profile.tmpl", gin.H{"User": user, "Galleries": gals, "myUser": myUser, "SameUser": SameUser})
	})

	rout.GET("/other/:o", func(c *gin.Context) {
		o := c.Param("o")
		c.String(200, nfkd(o))
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
	        c.String(500, "Error uploading file: " + err.Error())
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
            c.String(500, "Error reading file for validation: " + err.Error()) 
            c.Abort()
        }
        file.Seek(0, 0) //file.Read(buff) consumed our bytes, reset to start
        
        var content_type string = http.DetectContentType(vbuf)
	    if strings.HasPrefix(content_type, "image/") {
	        var extension string = strings.TrimPrefix(content_type, "image/")
	        
	        var scrubbed_image []byte = EraseGPS(file)
	        var thumb []byte
	        
	        var main_path, _ string = DefinePath("Noneedforthisparam", scrubbed_image, extension, "avatar")
	        thumb = CreateThumb(scrubbed_image, extension, false)
            err := UploadToCDN(bytes.NewReader(thumb), main_path)
            if err != nil {
                panic(err)
            }
            _, err = sqlUPDATEuserAVATAR.Exec(main_path, myUser.Username)
            if err != nil {
                panic(err)
            }
            
            c.JSON(201, gin.H{"url": StorageZoneRead+main_path})
            return
	    }
	    c.String(415, "Unsupported file type")
	})

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




