package main

import (
   "flag"
   "fmt"
   "hash/fnv"
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
   hash     uint64
   rev      int
}

// named by HTML 'id' field
type Section struct {
   highestRev  int
   highestPos  int
   highestNode *html.Node
   htmlnodes   []*HTMLNode
}

const (
   HashAttr = "data-xweb"
)

var (
   reformat  = flag.Bool("reformat", false, "reformat HTML files")
   htmlfiles []HTMLFile
   sections  = map[string]*Section{} // stored by element:id
   elements  = map[string]struct{}{
      "section": struct{}{},
      "header" : struct{}{},
      "footer" : struct{}{},
   }

   byhash = map[uint64]*html.Node{}
   byid   = map[string]*html.Node{}
)

func (dst *HTMLNode) update(src *Section) {
   dst.htmlfile.modified = true

   dst.node.FirstChild = src.highestNode.FirstChild
   dst.node.LastChild = src.highestNode.LastChild
   dst.node.Attr = src.highestNode.Attr
}

// needed before computing hash
func hashGetRemove(node *html.Node) (string, uint64, error) {
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

func hashUpdateChanged(node *html.Node) (bool, error) {
   id, oldhash, err := hashGetRemove(node)
   if err != nil {
      return false, fmt.Errorf("hashAdd: %w", err)
   }

   h := fnv.New64a()
   err = html.Render(h, node)
   if err != nil {
      return false, fmt.Errorf("hash: %w", err)
   }

   newhash := h.Sum64()
   node.Attr = append(node.Attr,
      html.Attribute{
         Key: "id",
         Val: id,
      },
   )
   node.Attr = append(node.Attr,
      html.Attribute{
         Key: HashAttr,
         Val: strconv.FormatUint(newhash, 16),
      },
   )

   return newhash != oldhash, nil
}

func build(htmlfile *HTMLFile, node *html.Node) error {
   // check if interesting element
   _, ok := elements[node.Data]

   if node.Type == html.ElementNode && ok {
      // hash HTML node and store by hash
      _, h, err := hashGetRemove(node) // FIXME
      if err != nil {
         return fmt.Errorf("build: %w", err)
      }

      _, ok := byhash[h]
      if ok {
         fmt.Println("present")
      }

      byhash[h] = node

      // previous code
      name := node.Data
      var rev, pos int

      for i, attr := range(node.Attr) {
         switch attr.Key {
         case "id":
            name += ":" + attr.Val
         case HashAttr:
            var err error
            rev, err = strconv.Atoi(attr.Val)
            if err != nil {
               fmt.Fprintf(os.Stderr, "error: malformed %s value '%s'\n", HashAttr, attr.Val)
               os.Exit(1)
            }
            pos = i
         }
      }

      section, ok := sections[name]
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
         sections[name] = &Section{
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

func recurse() error {
   err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
      if err != nil {
         fmt.Fprintf(os.Stderr, "warning: unable to open %s", path)
         return fmt.Errorf("recurse: %w:", err)
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
      err = build(&htmlfiles[i], htmlfiles[i].tree)
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

   if *reformat {
      dirty()
   }

/*   for id, section := range(sections) {
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
   }*/

   err = rerender()
   if err != nil {
      fmt.Fprintf(os.Stderr, "%v\n", err)
      os.Exit(1)
   }
}
