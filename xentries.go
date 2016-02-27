// XMLifies aspects of HTML documents
// for syndication feed post-processing.

package main

import (
	"bytes"
	xmlencoding "encoding/xml"
	"flag"
	"fmt"
	"github.com/andreastt/gitmeta"
	"github.com/moovweb/gokogiri"
	"github.com/moovweb/gokogiri/html"
	"github.com/moovweb/gokogiri/xml"
	"github.com/moovweb/gokogiri/xpath"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

var tag = flag.String("t", "", "only include entries with this tag")
var verbose = flag.Bool("v", false, "increase verbosity")

var blacklist = []string{"h1", "address", "footer"}

var titlePath = xpath.Compile(".//title")
var tagsPath = xpath.Compile(".//head/meta[@name='keywords']/@content")
var bodyPath = xpath.Compile(".//body")

type entries struct {
	Entries []*entry `xml:"entry"`
	Tag     string   `xml:"tag,attr,omitempty"`
}

type entry struct {
	Path    string    `xml:"path"`
	Ctime   time.Time `xml:"ctime"`
	Mtime   time.Time `xml:"mtime"`
	Title   string    `xml:"title"`
	Tags    []string  `xml:"tags>tag"`
	Summary chardata  `xml:"summary"`
}

func (e *entry) tagged(target string) bool {
	for _, t := range e.Tags {
		if t == target {
			return true
		}
	}
	return false
}

type chardata struct {
	Content []byte `xml:",innerxml"`
}

func createEntry(path string) (*entry, error) {
	repo, err := gitmeta.Repo(path)
	if err != nil {
		return nil, err
	}
	file := repo.File(path)

	entry := &entry{
		Path:  path,
		Ctime: file.Ctime(),
		Mtime: file.Mtime(),
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bs, err := ioutil.ReadFile(file.Path)
	if err != nil {
		return entry, err
	}
	doc, err := gokogiri.ParseHtml(bs)
	if err != nil {
		return entry, err
	}
	defer doc.Free()

	entry.Title = findString(doc, titlePath)
	entry.Tags = findTags(doc, tagsPath)
	entry.Summary = findContent(doc, bodyPath)

	return entry, nil
}

func find(doc *html.HtmlDocument, expr *xpath.Expression) ([]xml.Node, error) {
	return doc.Search(expr)
}

func findSingle(doc *html.HtmlDocument, expr *xpath.Expression) (xml.Node, error) {
	ns, err := find(doc, expr)
	if err != nil {
		return nil, err
	} else if len(ns) == 0 {
		return nil, fmt.Errorf("unable to find: %s", expr)
	}
	return ns[0], nil
}

func findString(doc *html.HtmlDocument, expr *xpath.Expression) string {
	el, err := findSingle(doc, expr)
	if err != nil {
		return ""
	}
	return el.Content()
}

func findTags(doc *html.HtmlDocument, expr *xpath.Expression) []string {
	// TODO(ato): Use strings.FieldFunc?
	ss := strings.Split(findString(doc, expr), ",")
	tags := make([]string, len(ss))
	for i, s := range ss {
		tags[i] = strings.TrimSpace(s)
	}
	return tags
}

func findContent(doc *html.HtmlDocument, expr *xpath.Expression) chardata {
	node, err := findSingle(doc, expr)
	if err != nil {
		return chardata{}
	}

	isBlacklisted := func(t xml.Node) bool {
		for _, i := range blacklist {
			if i == t.Name() {
				return true
			}
		}
		return false
	}

	els := make([]xml.Node, node.CountChildren())
	i := 0
	for n := node.FirstChild(); n != nil; n = n.NextSibling() {
		if n.NodeType() != xml.XML_ELEMENT_NODE || isBlacklisted(n) {
			continue
		}
		els[i] = n
		i++
	}

	var buf bytes.Buffer
	for _, el := range els {
		if el != nil {
			buf.WriteString(el.String())
		}
	}

	return chardata{[]byte("<![CDATA[" + buf.String() + "]]>")}
}

func marshal(els []*entry, tag string) []byte {
	entries := &entries{els, tag}
	out, err := xmlencoding.MarshalIndent(entries, "", "  ")
	if err != nil {
		die(err.Error())
	}
	return out
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		die("at least one document required")
	}

	// TODO: do we have to be explicit about len?
	// TODO: should this be an Entry _copy_ (not pointer?)
	entries := make([]*entry, flag.NArg())
	for i, doc := range flag.Args() {
		info("creating entry for %s", doc)
		entry, err := createEntry(doc)
		if err != nil {
			warn("unable to create entry %s: %s", doc, err)
		} else if len(*tag) == 0 || entry.tagged(*tag) {
			entries[i] = entry
		}
	}

	out := marshal(entries, *tag)
	fmt.Println(string(out))
}

func info(format string, v ...interface{}) {
	if *verbose {
		out(fmt.Sprintf(format, v...))
	}
}

func warn(format string, v ...interface{}) {
	s := fmt.Sprintf("warning: %s", format)
	out(fmt.Sprintf(s, v...))
}

func die(format string, v ...interface{}) {
	s := fmt.Sprintf("error: %s", format)
	out(fmt.Sprintf(s, v...))
	os.Exit(1)
}

func out(s string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], s)
}
