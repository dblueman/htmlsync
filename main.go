package main

import (
   "flag"
   "fmt"
   "io"
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

type HTMLNode struct {
   htmlfile *HTMLFile
   node     *html.Node
   rev      int
}

type Section struct {
   highestRev  int
   highestPos  int
   highestNode *html.Node
   htmlnodes   []*HTMLNode
}

const (
   CustomAttr = "data-revision"
)

var (
   reformat  = flag.Bool("reformat", false, "reformat HTML files")
   htmlfiles []HTMLFile
   sections  = map[string]*Section{} // stored by id
)

func (dst *HTMLNode) update(src *Section) {
   dst.htmlfile.modified = true

   dst.node.FirstChild = src.highestNode.FirstChild
   dst.node.LastChild = src.highestNode.LastChild
   dst.node.Attr = src.highestNode.Attr
}

func build(htmlfile *HTMLFile, node *html.Node) {
   if node.Type == html.ElementNode && node.Data == "section" {
      var id string
      var rev, pos int

      for i, attr := range(node.Attr) {
         switch attr.Key {
         case "id":
            id = attr.Val
         case CustomAttr:
            var err error
            rev, err = strconv.Atoi(attr.Val)
            if err != nil {
               fmt.Fprintf(os.Stderr, "error: malformed %s value '%s'; should be eg 'r7'\n", CustomAttr, attr.Val)
               os.Exit(1)
            }
            pos = i
         }
      }

      section, ok := sections[id]
      if ok {
         if rev > section.highestRev {
            section.highestRev = rev
            section.highestPos = pos
            section.highestNode = node
         }

         section.htmlnodes = append(section.htmlnodes, &HTMLNode{
            htmlfile: htmlfile,
            node:     node,
            rev:      rev,
         })
      } else {
         sections[id] = &Section{
            highestRev:  rev,
            highestPos:  pos,
            highestNode: node,
            htmlnodes:   []*HTMLNode{
               &HTMLNode{
                  htmlfile: htmlfile,
                  node:     node,
               },
            },
         }
      }
   }

   for child := node.FirstChild; child != nil; child = child.NextSibling {
		build(htmlfile, child)
	}
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

   // avoid loop variable aliasing
   for i := range(htmlfiles) {
      build(&htmlfiles[i], htmlfiles[i].tree)
   }

   for id, section := range(sections) {
      if section.highestRev == 0 {
         continue
      }

      fmt.Printf("found %d '%s' sections; latest revision %d\n", len(section.htmlnodes), id, section.highestRev)

      for _, htmlnode := range(section.htmlnodes) {
         // skip self
         if htmlnode.node == section.highestNode {
            continue
         }

         // skip uptodate sections
         if htmlnode.rev == section.highestRev {
            continue
         }

         htmlnode.update(section)
      }
   }

   var wrote []string

   // rerender modified files
   for _, htmlfile := range(htmlfiles) {
      if !htmlfile.modified {
         continue
      }

      err = htmlfile.render()
      if err != nil {
         return fmt.Errorf("top: %w", err)
      }

      wrote = append(wrote, htmlfile.name)
   }

   if len(wrote) > 0 {
      fmt.Printf("updated: %s\n", strings.Join(wrote, " "))
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
