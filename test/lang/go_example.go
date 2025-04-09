package main
 
import (
	"fmt"
	"time"
)

// Person represents a human being
type Person struct {
	Name    string
	Age     int
	Address *Address
}

// Address stores location information
type Address struct {
	Street  string
	City    string
	Country string
	ZipCode int
}

// SayHello is a method on Person that greets
func (p *Person) SayHello() string {
	return fmt.Sprintf("Hello, my name is %s and I am %d years old", p.Name, p.Age)
}

func main() {
	// Create a new person
	bob := &Person{
		Name: "Bob Smith",
		Age:  35,
		Address: &Address{
			Street:  "123 Main St",
			City:    "Anytown",
			Country: "USA",
			ZipCode: 12345,
		},
	}

	// Print a greeting
	fmt.Println(bob.SayHello())

	// Demonstrate some Go features
	for i := 0; i < 3; i++ {
		fmt.Printf("Counting: %d\n", i)
		time.Sleep(100 * time.Millisecond)
	}

	// Map example
	colors := map[string]string{
		"red":   "#ff0000",
		"green": "#00ff00",
		"blue":  "#0000ff",
	}

	for color, hex := range colors {
		fmt.Printf("Color %s has hex code %s\n", color, hex)
	}
}
