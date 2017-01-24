package main

import (
	"html/template"
	"net/http"

	"gopkg.in/mgo.v2/bson"

	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/url"

	"github.com/goincremental/negroni-sessions"
	"github.com/goincremental/negroni-sessions/cookiestore"
	"github.com/urfave/negroni"

	gmux "github.com/gorilla/mux"

	"log"

	"fmt"

	"github.com/maxwellhealth/bongo"
	"golang.org/x/crypto/bcrypt"
)

var Connections *bongo.Connection

type PageData struct {
	Books []BookDocument
	User  string
}

type LoginPageData struct {
	Error string
}

type SearchResult struct {
	Title  string `xml:"title,attr"`
	Author string `xml:"author,attr"`
	Year   string `xml:"hyr,attr"`
	ID     string `xml:"owi,attr"`
}

type ClassifySearchResponse struct {
	Results []SearchResult `xml:"works>work"`
}

type ClassifyBookResponse struct {
	BookData struct {
		Title  string `xml:"title,attr"`
		Author string `xml:"author,attr"`
		ID     string `xml:"owi,attr"`
	} `xml:"work"`
	Classification struct {
		MostPopular string `xml:"sfa,attr"`
	} `xml:"recommendations>ddc>mostPopular"`
}

type BookDocument struct {
	bongo.DocumentBase `bson:",inline"`
	Title              string
	Author             string
	Owi                string
	Classification     string
}

type UserDocument struct {
	bongo.DocumentBase `bson:",inline"`
	Username           string
	Secret             []byte
}

func MongoConnect() {
	config := &bongo.Config{
		ConnectionString: "127.0.0.1:27017", //or just localhost
		Database:         "library",
	}

	Connections, _ = bongo.Connect(config)
}

func getStringFromSession(r *http.Request, key string) string {
	var value string
	if val := sessions.GetSession(r).Get(key); val != nil {
		value = val.(string)
	}
	return value
}

func verifyUser(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.URL.Path == "/login" {
		next(w, r)
		return
	}

	if username := getStringFromSession(r, "User"); username != "" {
		if err := Connections.Collection("user").FindOne(bson.M{"username": username}, &UserDocument{}); err == nil {
			next(w, r)
			return
		}
	}

	http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
}

func main() {
	MongoConnect()

	n := negroni.Classic()
	n.Use(sessions.Sessions("library", cookiestore.New([]byte("secret123"))))
	n.Use(negroni.HandlerFunc(verifyUser))

	mux := gmux.NewRouter()

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var page LoginPageData

		if r.FormValue("register") != "" {
			// for registration

			err := Connections.Collection("user").FindOne(bson.M{"username": r.FormValue("username")}, &UserDocument{})

			if err == nil {
				page.Error = "Username already in the database. Please login instead"
			} else {
				secret, _ := bcrypt.GenerateFromPassword([]byte(r.FormValue("password")), bcrypt.DefaultCost)

				user := &UserDocument{
					Username: r.FormValue("username"),
					Secret:   secret,
				}

				err = Connections.Collection("user").Save(user)

				if err != nil {
					page.Error = err.Error()
				} else {
					sessions.GetSession(r).Set("User", user.Username)

					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
			}

		} else if r.FormValue("login") != "" {
			// for loging in

			user := &UserDocument{}
			err := Connections.Collection("user").FindOne(bson.M{"username": r.FormValue("username")}, user)

			if err != nil {
				page.Error = err.Error()
			} else {
				if err = bcrypt.CompareHashAndPassword(user.Secret, []byte(r.FormValue("password"))); err != nil {
					page.Error = err.Error()
					fmt.Println(err.Error())
				} else {
					sessions.GetSession(r).Set("User", user.Username)

					http.Redirect(w, r, "/", http.StatusFound)
					return
				}
			}
		}

		template := template.Must(template.ParseFiles("templates/login.html"))

		if err := template.ExecuteTemplate(w, "login.html", page); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("GET")

	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		sessions.GetSession(r).Set("User", nil)

		http.Redirect(w, r, "/login", http.StatusFound)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		books := findBooks()

		templates := template.Must(template.ParseFiles("templates/index.html"))

		if err := templates.ExecuteTemplate(w, "index.html", &PageData{Books: books, User: getStringFromSession(r, "User")}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("GET")

	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		var results []SearchResult
		var err error

		if results, err = search(r.FormValue("search")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(results); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("POST")

	mux.HandleFunc("/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		var book ClassifyBookResponse
		var bookDocument BookDocument
		var err error

		if book, err = find(gmux.Vars(r)["id"]); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		if bookDocument, err = insertBook(book); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// To send something as a json response it's good to use Encode method provided by json package to do so
		encoder := json.NewEncoder(w)
		if err = encoder.Encode(bookDocument); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}).Methods("PUT")

	mux.HandleFunc("/books/{owi}", func(w http.ResponseWriter, r *http.Request) {
		if err := removeBook(gmux.Vars(r)["owi"]); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}).Methods("DELETE")

	n.UseHandler(mux)
	n.Run(":8080")
}

func find(id string) (ClassifyBookResponse, error) {
	var c ClassifyBookResponse
	body, err := classifyAPI("http://classify.oclc.org/classify2/Classify?&summary=true&owi=" + url.QueryEscape(id))

	if err != nil {
		return ClassifyBookResponse{}, err
	}

	err = xml.Unmarshal(body, &c)

	return c, err
}

func search(query string) ([]SearchResult, error) {
	var c ClassifySearchResponse
	body, err := classifyAPI("http://classify.oclc.org/classify2/Classify?&summary=true&title=" + url.QueryEscape(query))

	if err != nil {
		return []SearchResult{}, err
	}

	err = xml.Unmarshal(body, &c)

	return c.Results, err
}

func classifyAPI(url string) ([]byte, error) {
	var resp *http.Response
	var err error

	if resp, err = http.Get(url); err != nil {
		return []byte{}, err
	}

	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func insertBook(book ClassifyBookResponse) (BookDocument, error) {
	bookDocument := BookDocument{}

	err := Connections.Collection("book").FindOne(bson.M{"owi": book.BookData.ID}, &bookDocument)

	if err == nil {
		log.Printf("Document already in the db [%s]", bookDocument.Owi)
		return bookDocument, err
	}

	log.Printf("Inserting new element [%s]", book.BookData.ID)

	err = Connections.Collection("book").Save(&BookDocument{
		Title:          book.BookData.Title,
		Author:         book.BookData.Author,
		Owi:            book.BookData.ID,
		Classification: book.Classification.MostPopular})

	return bookDocument, err
}

func removeBook(owi string) error {
	changeInfo, err := Connections.Collection("book").Delete(bson.M{"owi": owi})

	if err != nil {
		panic(err)
	}

	log.Printf("Deleted %d documents", changeInfo.Removed)

	return err
}

func findBooks() []BookDocument {
	result := Connections.Collection("book").Find(bson.M{})

	book := BookDocument{}

	var books []BookDocument

	for result.Next(&book) {
		books = append(books, book)
	}

	return books
}
