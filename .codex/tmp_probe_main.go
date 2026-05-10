package main
import (
  "context"
  "fmt"
  "github.com/KiriKirby/phytozome-go/internal/phytozome"
)
func main(){
  c:=phytozome.NewClient(nil)
  species,err:=c.FetchSpeciesCandidates(context.Background())
  if err!=nil { panic(err) }
  for i,s:= range species {
    if i>=20 { break }
    fmt.Printf("%d\t%s\t%s\t%d\n", i+1, s.GenomeLabel, s.JBrowseName, s.ProteomeID)
  }
}
