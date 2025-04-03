package main
 

 import (
  "bytes"
  "fmt"
 )
 

 func main() {
  var buffer bytes.Buffer
  buffer.WriteString("Initial data. More data here.")
  fmt.Println("Initial:", buffer.String())
 

  buffer.Truncate(7) // Keep only the first 7 bytes
  fmt.Println("Truncated:", buffer.String())
 }