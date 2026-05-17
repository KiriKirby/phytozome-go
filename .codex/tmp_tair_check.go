package main
import (
  "context"
  "fmt"
  "github.com/KiriKirby/phytozome-go/internal/tair"
)
func main(){
  c:=tair.NewClient(nil)
  versions,_:=c.FetchSpeciesCandidates(context.Background())
  for _,v:= range versions {
    if v.JBrowseName=="TAIR12" {
      fams, err := c.FetchFamilyCandidates(context.Background(), v)
      fmt.Println("families", len(fams), "err", err)
      rows, err := c.SearchKeywordRows(context.Background(), v, "AT1G01010")
      fmt.Println("keyword", len(rows), "err", err)
      return
    }
  }
  fmt.Println("no tair12")
}
