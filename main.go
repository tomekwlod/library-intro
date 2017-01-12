package main

import (
	"html/template"
	"net/http"

	"gopkg.in/mgo.v2/bson"
	"labix.org/v2/mgo"

	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/url"

	"github.com/codegangsta/negroni"
)

var (
	mgoSession   *mgo.Session
	databaseName = "library"
)

type Page struct {
	Books []BookDocument
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

func getMongoSession() *mgo.Session {
	if mgoSession == nil {
		var err error

		mgoSession, err = mgo.Dial("127.0.0.1:27017")

		if err != nil {
			panic(err)
		}
	}

	return mgoSession.Copy()
}

func main() {
	templates := template.Must(template.ParseFiles("templates/index.html"))

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := Page{}

		books, err := findBooks()

		if err != nil {
			panic(err)
		}

		p.Books = books

		if err := templates.ExecuteTemplate(w, "index.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

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
	})

	mux.HandleFunc("/books/add", func(w http.ResponseWriter, r *http.Request) {
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
	session := getMongoSession()
	defer session.Close()

	if err := session.Ping(); err != nil {
		return BookDocument{}, err
	}

	session.SetMode(mgo.Monotonic, true)

	collection := session.DB("library").C("book")

	counter, err := collection.Find(bson.M{"owi": book.BookData.ID}).Count()

	bookDocument := BookDocument{}
	if counter > 0 {
		// i am not sure if it's a correct logic to send no response to a frontend in this case
		panic(nil)
	}

	err = collection.Insert(&BookDocument{Title: book.BookData.Title, Author: book.BookData.Author, Owi: book.BookData.ID, Classification: book.Classification.MostPopular})

	if err != nil {
		return BookDocument{}, err
	}

	err = collection.Find(bson.M{"owi": book.BookData.ID}).One(&bookDocument)

	if err != nil {
		return BookDocument{}, err
	}

	return bookDocument, err
}

func findBooks() ([]BookDocument, error) {
	session := getMongoSession()
	defer session.Close()
	var results []BookDocument
	var err error

	if err = session.Ping(); err != nil {
		return []BookDocument{}, err
	}

	session.SetMode(mgo.Monotonic, true)

	collection := session.DB("library").C("book")

	err = collection.Find(bson.M{}).All(&results)

	if err != nil {
		return []BookDocument{}, err
	}

	return results, err
}
