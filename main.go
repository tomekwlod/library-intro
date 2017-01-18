package main

import (
	"html/template"
	"net/http"

	"gopkg.in/mgo.v2/bson"

	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/url"

	"github.com/codegangsta/negroni"
	gmux "github.com/gorilla/mux"

	"log"

	"github.com/maxwellhealth/bongo"
)

var Connections *bongo.Connection

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

func MongoConnect() {
	config := &bongo.Config{
		ConnectionString: "127.0.0.1:27017", //or just localhost
		Database:         "library",
	}

	Connections, _ = bongo.Connect(config)
}

func main() {
	MongoConnect()

	templates := template.Must(template.ParseFiles("templates/index.html"))

	mux := gmux.NewRouter()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		books := findBooks()

		if err := templates.ExecuteTemplate(w, "index.html", books); err != nil {
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

	n := negroni.Classic()
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
