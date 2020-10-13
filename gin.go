package main

import (
	"log"
	"fmt"
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
)

func AssembleDriverStr() string {
	return fmt.Sprintf("%v:%v@/%v", Config.DBun, Config.DBpass, Config.DBdb)
}

type User struct {
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
			c.Set("myUser", myUser)
		} else {
			c.Set("myUser", nil)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmt.Printf("conf: %v\n", Config)
	nfkd := norm.NFKD.String

	db, err := sql.Open("mysql", AssembleDriverStr())
	if err != nil { log.Fatal(err) }
	defer db.Close()

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
		c.HTML(http.StatusOK, "register.tmpl", gin.H{})
	})
	rout.POST("/register", func(c *gin.Context) {
		username := c.PostForm("username")
		if utf8.ValidString(username) {
			cleanUN := nfkd(username)
			var Result string
			notok := sqlSELECTuserNAME.QueryRow(cleanUN).Scan(&Result)
			if notok != sql.ErrNoRows {
				if notok != nil { log.Fatal(notok) }
				c.Abort()
			} else {	//error IS "no rows found"
				display_name := c.DefaultPostForm("display", username)
				username = cleanUN
				passw := []byte(c.PostForm("password"))
				hash, err := bcrypt.GenerateFromPassword(passw, Config.PassCost)
				if err != nil { log.Panic(err) }
				_, err2 := sqlINSERTuser.Exec(nil, username, display_name, hash)	//Exec does not return useful information related to the results of the query, therefore I only consider it appropriate for INSERT statements.
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

	rout.GET("/upload", func(c *gin.Context) {
		//if registered
		c.HTML(http.StatusOK, "upload_form.tmpl", gin.H{})
	})
	rout.POST("/upload", func(c *gin.Context) {
		photo, _ := c.FormFile("photo")
		file, err := photo.Open()
		if err != nil { 
			c.String(500, "Error uploading file: " + err.Error()) 
			c.Abort()
		}
		defer file.Close()
		buff := make([]byte, 512) // https://stackoverflow.com/a/38175140/12514997
		_, err = file.Read(buff)
		if err != nil { 
			c.String(500, "Error reading file for validation: " + err.Error()) 
			c.Abort()
		}
		
		if strings.HasPrefix(http.DetectContentType(buff), "image/") {
		
		} else {
			c.String(415, "Unsupported file type") //415 -> Media type unsupported
			c.Abort()
		}

		c.String(200, "OK")
	})

	rout.GET("/profiles/:path", func(c *gin.Context) {
		myUser, _ := c.Get("myUser")

		path := c.Param("path")
		var user User
		err := sqlSELECTuserNAME.QueryRow(path).Scan(&user.id, &user.Username, &user.Displayname, &user.Avatar)
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
		/*
		photos := make([]Photo, 0)
		for rows.Next() {
			var photo Photo
			if err := rows.Scan(&photo.Owner, &photo.Reference, &photo.Id, &photo.Description); err != nil {
				log.Fatal(err)
			}
			photos = append(photos, photo)
		}
		*/

		var SameUser bool
		if myUser.(User).id == user.id {
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
    /*
	rout.POST("/update_avatar", func(c *gin.Context) {
		url := c.PostForm("url")
		myUser, _ := c.Get("myUser")
		if myUser != nil {//crunge
		    //unimplemented
		}
	})
    */
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




