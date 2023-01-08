package main

import (
   "flag"
   "fmt"
   "io"
   "os"
   "path/filepath"
   "regexp"
   "strconv"
   "strings"

   "golang.org/x/net/html"
)

type HTMLFile struct {
   file     *os.File
   tree     *html.Node
   modified bool
}

type Section struct {
   highestRev  int
   highestNode *html.Node
   nodes       []*html.Node
}

var (
   reformat  = flag.Bool("reformat", false, "reformat HTML files")
   htmlfiles []HTMLFile
   sections  = map[string]*Section{} // stored by id
   revRe     = regexp.MustCompile(`^r\d+$`)
)

func getRev(name string) int {
   match := revRe.MatchString(name)
   if !match {
      fmt.Fprintf(os.Stderr, "error: malformed data-xweb value '%s'; should be eg 'r7'\n", name)
      os.Exit(1)
   }

   val, err := strconv.Atoi(name[1:])
   if err != nil {
      panic(err)
   }

   return val
}

func build(node *html.Node) {
   if node.Type == html.ElementNode && node.Data == "section" {
      var id string
      var rev int

      for _, attr := range(node.Attr) {
         switch attr.Key {
         case "id":
            id = attr.Val
//            fmt.Printf("section id '%s'\n", attr.Val)
         case "data-xweb":
            rev = getRev(attr.Val)
//            fmt.Printf("data-xweb '%s'\n", attr.Val)
         }
      }

      section, ok := sections[id]
      if ok {
         if rev > section.highestRev {
            section.highestRev = rev
            section.highestNode = node
         }

         section.nodes = append(section.nodes, node)
      } else {
         sections[id] = &Section{
            highestRev:  rev,
            highestNode: node,
            nodes:       []*html.Node{node},
         }
      }
   }

   for child := node.FirstChild; child != nil; child = child.NextSibling {
		build(child)
	}
}

func (htmlfile *HTMLFile) build() error {
   build(htmlfile.tree)
   return nil
}

func parse(path string) error {
   var htmlfile HTMLFile
   var err error

   htmlfile.file, err = os.OpenFile(path, os.O_RDWR, 0o644)
   if err != nil {
      return fmt.Errorf("parse: %w", err)
   }

   htmlfile.tree, err = html.Parse(htmlfile.file)
   if err != nil {
      return fmt.Errorf("parse: %w", err)
   }

   htmlfiles = append(htmlfiles, htmlfile)
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

   return nil
}

func top() error {
   err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
      if err != nil {
         fmt.Fprintf(os.Stderr, "warning: unable to open %s", path)
         return fmt.Errorf("top: %w:", err)
      }

      if !strings.HasSuffix(path, ".html") {
         return nil
      }

      err = parse(path)
      if err != nil {
         return fmt.Errorf("top: %w", err)
      }

      return nil
   })

   if *reformat {
      for _, htmlfile := range(htmlfiles) {
         err = htmlfile.render()
         if err != nil {
            return fmt.Errorf("top: %w", err)
         }
      }

      return nil
   }

   for _, htmlfile := range(htmlfiles) {
      err = htmlfile.build()
      if err != nil {
         return fmt.Errorf("top: %w", err)
      }
   }

   for id, section := range(sections) {
      fmt.Printf("section '%s', highestRev %d, %d nodes\n", id, section.highestRev, len(section.nodes))
   }

   return err
}

func main() {
   flag.Usage = func() {
      fmt.Fprintln(os.Stderr, "usage: xweb [option]")
      flag.PrintDefaults()
   }

   flag.Parse()

   err := top()
   if err != nil {
      fmt.Fprintf(os.Stderr, "%v\n", err)
      os.Exit(1)
   }
}
