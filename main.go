package main

import (
   "flag"
   "fmt"
   "hash/fnv"
   "io"
   "math/rand"
   "os"
   "path/filepath"
   "strconv"
   "strings"

   "golang.org/x/net/html"
)

type HTMLFile struct {
   name     string
   file     *os.File
   tree     *html.Node
   modified bool
}

type Section struct {
   id       string
   oldhash  uint64
   newhash  uint64
   htmlnode *html.Node
   htmlfile *HTMLFile
}

const (
   HashAttr = "data-xweb"
)

var (
   reformatFlag = flag.Bool("reformat", false, "reformat HTML files")
   htmlfiles = []*HTMLFile{}
   elements  = map[string]struct{}{
      "section": struct{}{},
      "header" : struct{}{},
      "footer" : struct{}{},
   }
   sectionsByHash = map[uint64][]*Section{}
   sectionsByID   = map[string][]*Section{}
)

func (dst *Section) update(src *Section) {
   dst.htmlfile.modified = true

   dst.htmlnode.FirstChild = src.htmlnode.FirstChild
   dst.htmlnode.LastChild = src.htmlnode.LastChild
   dst.htmlnode.Attr = src.htmlnode.Attr
}

func randID() string {
   return fmt.Sprintf("%06d", rand.Intn(1000000))
}

// needed before computing hash
func hashIdGetRemove(node *html.Node) (string, uint64, error) {
   var id string
   var hash uint64
   var err error

   for i := len(node.Attr)-1; i >= 0; i-- {
      switch node.Attr[i].Key {
      // skip removal
      default:
         continue
      case HashAttr:
      hash, err = strconv.ParseUint(node.Attr[i].Val, 16, 64)
      if err != nil {
         return "", 0, fmt.Errorf("hashGetRemove: malformed hash %s", node.Attr[i].Val)
      }
      case "id":
         id = node.Attr[i].Val
      }

      node.Attr = append(node.Attr[:i], node.Attr[i+1:]...)
   }

   return id, hash, nil
}

// hash and id must be removed previously
func hashCompute(node *html.Node) (uint64, error) {
   h := fnv.New64a()
   err := html.Render(h, node)
   if err != nil {
      return 0, fmt.Errorf("hash: %w", err)
   }

   return h.Sum64(), nil
}

func hashIDAdd(node *html.Node, hash uint64, id string) {
   // id and hash as first attributes
   node.Attr = append([]html.Attribute{
      html.Attribute{
         Key: "id",
         Val: id,
      },
      html.Attribute{
         Key: HashAttr,
         Val: strconv.FormatUint(hash, 16),
      }},
      node.Attr...,
   )
}

func (s *Section) setID(id string) {
   if id == "" {
      panic("null ID")
   }

   s.id = id
   s.htmlfile.modified = true

   for _, attr := range(s.htmlnode.Attr) {
      if attr.Key == "id" {
         attr.Val = id
         return
      }
   }

   // doesn't exist; add
   s.htmlnode.Attr = append(s.htmlnode.Attr, html.Attribute{
      Key: "id",
      Val: id,
   })
}

func build(htmlfile *HTMLFile, node *html.Node) error {
   // add to HMTLfile list for dirtying
   htmlfiles = append(htmlfiles, htmlfile)

   // check if interesting element
   _, ok := elements[node.Data]

   if node.Type == html.ElementNode && ok {
      // hash HTML node and store by hash
      id, oldhash, err := hashIdGetRemove(node)
      if err != nil {
         return fmt.Errorf("build: %w", err)
      }

      newhash, err := hashCompute(node)
      if err != nil {
         return fmt.Errorf("build: %w", err)
      }

      if id == "" {
         id = strconv.FormatUint(newhash, 16)
      }

      hashIDAdd(node, newhash, id)

      // must update hash
      if newhash != oldhash {
         htmlfile.modified = true
      }

      section := Section{
         id,
         oldhash,
         newhash,
         node,
         htmlfile,
      }

      sections := sectionsByHash[newhash]
      sections = append(sections, &section)
      sectionsByHash[oldhash] = sections

      sections = sectionsByID[id]
      sections = append(sections, &section)
      sectionsByID[id] = sections
   }

   for child := node.FirstChild; child != nil; child = child.NextSibling {
		err := build(htmlfile, child)
      if err != nil {
         return err
      }
	}

   return nil
}

func parse(path string) error {
   htmlfile := HTMLFile{
      name: path,
   }

   var err error

   htmlfile.file, err = os.OpenFile(path, os.O_RDWR, 0o644)
   if err != nil {
      return fmt.Errorf("parse: %w", err)
   }

   htmlfile.tree, err = html.Parse(htmlfile.file)
   if err != nil {
      return fmt.Errorf("parse: %w", err)
   }

   htmlfiles = append(htmlfiles, &htmlfile)
   return nil
}

func (htmlfile *HTMLFile) render() error {
   _, err := htmlfile.file.Seek(0, io.SeekStart)
   if err != nil {
      return fmt.Errorf("render: %w", err)
   }

   err = htmlfile.file.Truncate(0)
   if err != nil {
      return fmt.Errorf("render: %w", err)
   }

   err = html.Render(htmlfile.file, htmlfile.tree)
   if err != nil {
      return fmt.Errorf("render: %w", err)
   }

   htmlfile.modified = false
   return nil
}

func recurse() error {
   err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
      if err != nil {
         fmt.Fprintf(os.Stderr, "warning: unable to open %s", path)
         return fmt.Errorf("recurse: %w", err)
      }

      if !strings.HasSuffix(path, ".html") {
         return nil
      }

      err = parse(path)
      if err != nil {
         return fmt.Errorf("recurse: %w", err)
      }

      return nil
   })

   // avoid loop variable aliasing
   for i := range(htmlfiles) {
      err = build(htmlfiles[i], htmlfiles[i].tree)
      if err != nil {
         return fmt.Errorf("top: %w", err)
      }
   }

   return nil
}

func dirty() {
   for _, htmlfile := range(htmlfiles) {
      htmlfile.modified = true
   }
}

func rerender() error {
   var wrote []string

   // rerender modified files
   for _, htmlfile := range(htmlfiles) {
      if !htmlfile.modified {
         continue
      }

      err := htmlfile.render()
      if err != nil {
         return fmt.Errorf("rerender: %w", err)
      }

      wrote = append(wrote, htmlfile.name)
   }

   if len(wrote) > 0 {
      fmt.Printf("updated: %s\n", strings.Join(wrote, " "))
   }

   return nil
}

func reformat() {
   // sections with same hash are updated with the same id
   for _, sections := range(sectionsByHash) {
      var shortestID string

      // find shortest ID
      for _, section := range(sections) {
         if shortestID == "" || len(section.id) < len(shortestID) {
            shortestID = section.id
         }
      }

      for _, section := range(sections) {
         if section.id != shortestID {
            section.setID(shortestID)
         }
      }
   }

   // sections with different hash but same id are given unique id
   for _, sections := range(sectionsByID) {
      firstHash := sections[0].newhash

      for i := 1; i < len(sections); i++ {
         if sections[i].newhash != firstHash {
            newId := sections[i].id + "-" + randID()
            sections[i].setID(newId)
         }
      }
   }

   // force all files to be rerendered
   dirty()
}

func mirror() error {
   for id, sections := range(sectionsByID) {
      changed := []*Section{}

      for _, section := range(sections) {
         if section.oldhash != section.newhash {
            changed = append(changed, section)
         }
      }

      if len(changed) == 0 {
         continue
      }

      if len(changed) > 1 {
         for i, section := range(changed) {
            fmt.Printf("-- changed '%s' section %d/%d ------------\n\n", id, i, len(changed)-1)
            err := html.Render(os.Stdout, section.htmlnode)
            if err != nil {
               return fmt.Errorf("mirror: %w", err)
            }

            fmt.Println("\n")
         }

again:
         fmt.Printf("-- which section 0-%d should be used? ", len(changed)-1)
         var selection int
         n, err := fmt.Fscanf(os.Stdin, "%d", &selection)
         if n != 1 || err != nil || selection < 0 || selection > (len(changed)-1) {
            goto again
         }

         changed = []*Section{changed[selection]}
      }

      // use changed[0]
      for i := 1; i < len(sections); i++ {
         sections[i].update(changed[0])
      }
   }

   return nil
}

func main() {
   flag.Usage = func() {
      fmt.Fprintln(os.Stderr, "usage: xweb [option]")
      flag.PrintDefaults()
   }

   flag.Parse()

   err := recurse()
   if err != nil {
      fmt.Fprintf(os.Stderr, "%v\n", err)
      os.Exit(1)
   }

   if *reformatFlag {
      reformat()
   } else {
      err = mirror()
      if err != nil {
         fmt.Fprintf(os.Stderr, "%v\n", err)
         os.Exit(1)
      }
   }

   err = rerender()
   if err != nil {
      fmt.Fprintf(os.Stderr, "%v\n", err)
      os.Exit(1)
   }
}
