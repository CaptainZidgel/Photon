package main

import (
    "regexp"
    "time"
    "errors"
    "strings"
    "os"
    "io"
    "encoding/csv"
    "log"
)

//https://www.cloudhadoop.com/2018/12/go-example-program-to-check-string_13.html
var isANumeric = regexp.MustCompile(`^[a-zA-Z0-9_-]*$`).MatchString

func genericRegex(pattern, s string) bool {
    return regexp.MustCompile(pattern).MatchString(s)
}

func VerifyUsername(username string) bool {
    return !(username[0] == '_' || !isANumeric(username) || len(username) < 5)
}

//the template form already has these checks in a javascript function but that could be disabled client side
func VerifyPasswordBasic(create bool, newpass string, conf string, old string) error {
    if len(newpass) < 8 {
        return errors.New("Password must be 8 characters or longer")
    }
    if newpass != conf {
        return errors.New("New Password and Confirm New Password must match")
    }
    if genericRegex(`^[a-zA-Z]*$`, newpass) {
        return errors.New("Password must contain a number")
    }
    if genericRegex(`^[0-9]*$`, newpass) {
        return errors.New("Password must contain a letter")
    }
    if newpass == strings.ToLower(newpass) || newpass == strings.ToUpper(newpass) {
        return errors.New("Password must contain a mix of lowercase and uppercase letters")
    }
    if !create && (old == newpass) {
        return errors.New("Old password and new password cannot be the same")
    }
    return nil
}

func NowDateString() string {
    t := time.Now()
    ts := t.Format("06-01-02 15:04:05")
    return ts
}


func ReadCSV(fpath string) []string {
    f, err := os.Open(fpath)
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()
    
    var names []string
    rows := csv.NewReader(f)
    for {
        record, err := rows.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Fatal(err)
        }
        names = append(names, record[0])
    }
    return names
}

func ReservedSet(arr []string) map[string]bool {
    names := make(map[string]bool)
    for _, v := range arr {
        names[v] = true
    }
    return names
}

func ReservedNames(fpath string) map[string]bool {
    var arr []string = ReadCSV(fpath)
    return ReservedSet(arr)
}

