package main
import (
  "bytes"
  "fmt"
  "io"
  "net/http"
  "time"
)
func do(name string, req *http.Request){
  c := &http.Client{Timeout:30*time.Second}
  resp, err := c.Do(req)
  if err != nil { fmt.Println(name, "ERR", err); return }
  defer resp.Body.Close()
  b,_:=io.ReadAll(io.LimitReader(resp.Body, 600))
  fmt.Println(name, resp.Status, resp.Header.Get("Content-Type"))
  fmt.Println(string(b))
}
func main(){
  req1,_:=http.NewRequest("GET", "https://www.arabidopsis.org/servlets/TairObject?type=locus&name=AT1G01010", nil)
  do("plain-get", req1)
  req2,_:=http.NewRequest("GET", "https://www.arabidopsis.org/servlets/TairObject?type=locus&name=AT1G01010", nil)
  req2.Header.Set("User-Agent", "Mozilla/5.0")
  req2.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
  do("ua-get", req2)
  req3,_:=http.NewRequest("POST", "https://www.arabidopsis.org/api/search/gene", bytes.NewBufferString(`{"key":"AT1G01010"}`))
  req3.Header.Set("Content-Type", "application/json")
  do("plain-post", req3)
  req4,_:=http.NewRequest("POST", "https://www.arabidopsis.org/api/search/gene", bytes.NewBufferString(`{"key":"AT1G01010"}`))
  req4.Header.Set("User-Agent", "Mozilla/5.0")
  req4.Header.Set("Accept", "application/json, text/plain, */*")
  req4.Header.Set("Origin", "https://www.arabidopsis.org")
  req4.Header.Set("Referer", "https://www.arabidopsis.org/")
  req4.Header.Set("Content-Type", "application/json")
  do("ua-post", req4)
}
