package main

import (
	"fmt"
	"html/template"
	"net/http"

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

	fmt.Println(http.ListenAndServe(":8080", nil))
}

type ClassifySearchResponse struct {
	Results []SearchResult `xml:"works>work"`
}

func search(query string) ([]SearchResult, error) {
	var resp *http.Response
	var err error

	if resp, err = http.Get("http://classify.oclc.org/classify2/Classify?&summary=true&title=" + url.QueryEscape(query)); err != nil {
		return []SearchResult{}, err
	}

	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		return []SearchResult{}, err
	}

	var c ClassifySearchResponse
	err = xml.Unmarshal(body, &c)

	return c.Results, err
}
