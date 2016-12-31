package main

import (
	"fmt"
	"html/template"
	"net/http"

	"labix.org/v2/mgo"

	"encoding/json"
)

type Page struct {
	Name        string
	MongoStatus bool
}

type SearchResult struct {
	Title  string
	Author string
	Year   string
	ID     string
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
		results := []SearchResult{
			SearchResult{"Forest Gump", "Tomasz Wlodarczyk", "1983", "12321"},
			SearchResult{"Rango", "Johnny Cash", "1992", "3493"},
			SearchResult{"Titanic", "Nick Walter", "1983", "1232431"},
		}

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(results); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	fmt.Println(http.ListenAndServe(":8080", nil))
}
