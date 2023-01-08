package main

import (
   "flag"
   "fmt"
   "io"
   "os"
   "path/filepath"
   "strings"

   "golang.org/x/net/html"
)

type HTMLFile struct {
   file     *os.File
   tree     *html.Node
   modified bool
}

type Section struct {
   highest int
   nodes   []*html.Node
}

var (
   reformat  = flag.Bool("reformat", false, "reformat HTML files")
   htmlfiles []HTMLFile
   sections  map[string]Section // stored by id
)

func search(node *html.Node) {
   if node.Type == html.ElementNode && node.Data == "section" {
      for _, attr := range(node.Attr) {
         switch attr.Key {
         case "id":
            fmt.Printf("section id '%s'\n", attr.Val)
         case "data-xweb":
            fmt.Printf("data-xweb '%s'\n", attr.Val)
         }
      }
   }

   for child := node.FirstChild; child != nil; child = child.NextSibling {
		search(child)
	}
}

func (htmlfile *HTMLFile) search() error {
   search(htmlfile.tree)
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
      err = htmlfile.search()
      if err != nil {
         return fmt.Errorf("top: %w", err)
      }
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
