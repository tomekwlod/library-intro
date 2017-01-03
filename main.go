package main

import (
	"fmt"
	"html/template"
	"net/http"

	"gopkg.in/mgo.v2/bson"
	"labix.org/v2/mgo"

	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/url"
)

type Page struct {
	Name        string
	MongoStatus bool
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
	ID             bson.ObjectId `bson:"_id,omitempty"`
	Title          string
	Author         string
	Owi            string
	Classification string
}

func main() {
	templates := template.Must(template.ParseFiles("templates/index.html"))

	session, err := mgo.Dial("127.0.0.1:27017")
	if err != nil {
		panic(err)
	}

	defer session.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := Page{Name: "Tomek"}

		if name := r.FormValue("name"); name != "" {
			p.Name = name
		}

		p.MongoStatus = session.Ping() == nil

		if err := templates.ExecuteTemplate(w, "index.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		var results []SearchResult
		var err error

		if results, err = search(r.FormValue("search")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(results); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/books/add", func(w http.ResponseWriter, r *http.Request) {
		var book ClassifyBookResponse
		var bookDocument BookDocument
		var err error

		if book, err = find(r.FormValue("id")); err != nil {
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
	})

	fmt.Println(http.ListenAndServe(":8080", nil))
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
	session, err := mgo.Dial("127.0.0.1:27017")

	if err != nil {
		panic(err)
	}

	defer session.Close()

	if err = session.Ping(); err != nil {
		return BookDocument{}, err
	}

	session.SetMode(mgo.Monotonic, true)

	c := session.DB("library").C("book")

	var counter int
	counter, err = c.Find(bson.M{"owi": book.BookData.ID}).Count()

	bookDocument := BookDocument{}
	if counter > 0 {
		err = c.Find(bson.M{"owi": book.BookData.ID}).One(&bookDocument)

		return bookDocument, err
	}

	err = c.Insert(&BookDocument{Title: book.BookData.Title, Author: book.BookData.Author, Owi: book.BookData.ID, Classification: book.Classification.MostPopular})

	if err != nil {
		return BookDocument{}, err
	}

	err = c.Find(bson.M{"owi": book.BookData.ID}).One(&bookDocument)

	if err != nil {
		return BookDocument{}, err
	}

	return bookDocument, err
}