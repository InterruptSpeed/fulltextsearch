package main

import (
	"compress/gzip"
	"crypto/sha1"
	"encoding/gob"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"unicode"

	snowballeng "github.com/kljensen/snowball/english"
)

type abstract struct {
	Documents []document `xml:"doc"`
}

type document struct {
	Title   string `xml:"title"`
	URL     string `xml:"url"`
	Text    string `xml:"abstract"`
	URLSHA1 []byte
	ID      int
}

func loadDocuments(path string) ([]document, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	decoder := xml.NewDecoder(gz)

	abs := new(abstract)
	err = decoder.Decode(&abs)
	if err != nil {
		return nil, err
	}

	docs := abs.Documents
	for i := range docs {
		h := sha1.New()
		io.WriteString(h, docs[i].URL)
		docs[i].URLSHA1 = h.Sum(nil)

		docs[i].ID = i

		//file, _ := xml.MarshalIndent(docs[i], "", " ")
		//_ = ioutil.WriteFile(fmt.Sprintf("docs/%d.xml", docs[i].ID), file, 0644)
	}
	return docs, nil
}

func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		// Split on any character that is not a letter or a number.
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func lowercaseFilter(tokens []string) []string {
	r := make([]string, len(tokens))
	for i, token := range tokens {
		r[i] = strings.ToLower(token)
	}
	return r
}

var stopwords = map[string]struct{}{ // I wish Go had built-in sets.
	"a": {}, "and": {}, "be": {}, "have": {}, "i": {},
	"in": {}, "of": {}, "that": {}, "the": {}, "to": {},
}

func stopwordFilter(tokens []string) []string {
	r := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, ok := stopwords[token]; !ok {
			r = append(r, token)
		}
	}
	return r
}

func stemmerFilter(tokens []string) []string {
	r := make([]string, len(tokens))
	for i, token := range tokens {
		r[i] = snowballeng.Stem(token, false)
	}
	return r
}

func analyze(text string) []string {
	tokens := tokenize(text)
	tokens = lowercaseFilter(tokens)
	tokens = stopwordFilter(tokens)
	tokens = stemmerFilter(tokens)
	return tokens
}

type index map[string][]int

func (idx index) add(docs []document) {
	for _, doc := range docs {
		for _, token := range analyze(doc.Text) {
			ids := idx[token]
			if ids != nil && ids[len(ids)-1] == doc.ID {
				// Don't add same ID twice.
				continue
			}
			idx[token] = append(ids, doc.ID)
		}
	}
}

func intersection(a []int, b []int) []int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	r := make([]int, 0, maxLen)
	var i, j int
	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			i++
		} else if a[i] > b[j] {
			j++
		} else {
			r = append(r, a[i])
			i++
			j++
		}
	}
	return r
}

func (idx index) search(text string) []int {
	var r []int
	for _, token := range analyze(text) {
		if ids, ok := idx[token]; ok {
			if r == nil {
				r = ids
			} else {
				r = intersection(r, ids)
			}
		} else {
			// Token doesn't exist.
			return nil
		}
	}
	return r
}

func main() {
	idxFilename := "enwiki.idx"
	idx := make(index)

	if _, err := os.Stat(idxFilename); err == nil {
		// path/to/whatever exists
		log.Println("full text search index exists; using...")
		decodeFile, err := os.Open(idxFilename)
		if err != nil {
			panic(err)
		}
		defer decodeFile.Close()

		// Create a decoder
		decoder := gob.NewDecoder(decodeFile)

		// Decode -- We need to pass a pointer otherwise accounts2 isn't modified
		decoder.Decode(&idx)
	} else if os.IsNotExist(err) {
		// path does *not* exist, so build index and save
		log.Println("full text search index does not exist; rebuilding...")

		docs, err := loadDocuments("enwiki-latest-abstract1.xml.gz")
		if err != nil {
			log.Fatal(err)
			return
		}

		//idx.add([]document{{ID: 1, Text: "A donut on a glass plate. Only the donuts."}})
		//idx.add([]document{{ID: 2, Text: "donut is a donut"}})
		idx.add(docs)

		// Create a file for IO
		encodeFile, err := os.Create(idxFilename)
		if err != nil {
			panic(err)
		}

		// Since this is a binary format large parts of it will be unreadable
		encoder := gob.NewEncoder(encodeFile)

		// Write to the file
		if err := encoder.Encode(idx); err != nil {
			panic(err)
		}
		encodeFile.Close()
	} else {
		log.Fatal(err)
		// Schrodinger: file may or may not exist. See err for details.

		// Therefore, do *NOT* use !os.IsNotExist(err) to test for file existence

	}

	r := idx.search("small wild cat")

	fmt.Println(r)

	// this part is really slow but there isn't a clear way to index
	// into the original xml file without reading it entirely
	//docs, err := loadDocuments("enwiki-latest-abstract1.xml.gz")
	//if err != nil {
	//	log.Fatal(err)
	//	return
	//}
	//for _, id := range r {
	//	doc := docs[id]
	//	fmt.Printf("[%d]\t%s\n", id, doc.Text)
	//}
}
