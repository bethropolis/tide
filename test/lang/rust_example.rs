//! A simple Rust program demonstrating various language features

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

/// Represents a book in our library
#[derive(Debug, Clone)]
struct Book {
    title: String,
    author: String,
    year: u16,
    isbn: String,
    genres: Vec<String>,
}

impl Book {
    /// Creates a new Book instance
    fn new(title: &str, author: &str, year: u16, isbn: &str) -> Self {
        Book {
            title: title.to_string(),
            author: author.to_string(),
            year,
            isbn: isbn.to_string(),
            genres: Vec::new(),
        }
    }
    
    /// Adds a genre to the book
    fn add_genre(&mut self, genre: &str) {
        self.genres.push(genre.to_string());
    }
    
    /// Returns true if the book is considered a classic (over 50 years old)
    fn is_classic(&self) -> bool {
        // Current year - published year > 50
        2023 - self.year as u32 > 50
    }
}

/// Trait for items that can be borrowed from the library
trait Borrowable {
    fn get_id(&self) -> &str;
    fn is_available(&self) -> bool;
    fn borrow_item(&mut self) -> Result<(), &'static str>;
    fn return_item(&mut self) -> Result<(), &'static str>;
}

impl Borrowable for Book {
    fn get_id(&self) -> &str {
        &self.isbn
    }
    
    fn is_available(&self) -> bool {
        // For this example, assume books published before 1900 are reference only
        self.year >= 1900
    }
    
    fn borrow_item(&mut self) -> Result<(), &'static str> {
        if !self.is_available() {
            return Err("This book cannot be borrowed");
        }
        Ok(())
    }
    
    fn return_item(&mut self) -> Result<(), &'static str> {
        Ok(())
    }
}

/// A library that manages a collection of books
struct Library {
    books: HashMap<String, Book>,
    name: String,
}

impl Library {
    fn new(name: &str) -> Self {
        Library {
            name: name.to_string(),
            books: HashMap::new(),
        }
    }
    
    fn add_book(&mut self, book: Book) {
        self.books.insert(book.isbn.clone(), book);
    }
    
    fn get_book(&self, isbn: &str) -> Option<&Book> {
        self.books.get(isbn)
    }
    
    fn get_books_by_author(&self, author: &str) -> Vec<&Book> {
        self.books.values()
            .filter(|book| book.author.contains(author))
            .collect()
    }
    
    fn count_books(&self) -> usize {
        self.books.len()
    }
}

fn main() {
    // Create some books
    let mut book1 = Book::new(
        "The Great Gatsby", 
        "F. Scott Fitzgerald", 
        1925, 
        "9780743273565"
    );
    book1.add_genre("Classic");
    book1.add_genre("Fiction");
    
    let mut book2 = Book::new(
        "To Kill a Mockingbird", 
        "Harper Lee", 
        1960, 
        "9780061120084"
    );
    book2.add_genre("Fiction");
    
    let mut book3 = Book::new(
        "1984", 
        "George Orwell", 
        1949, 
        "9780451524935"
    );
    book3.add_genre("Dystopian");
    
    // Create a library
    let mut library = Library::new("City Central Library");
    library.add_book(book1);
    library.add_book(book2);
    library.add_book(book3);
    
    // Print information about the library
    println!("Library: {}", library.name);
    println!("Number of books: {}", library.count_books());
    
    // Demonstrate some pattern matching
    if let Some(book) = library.get_book("9780061120084") {
        match book.year {
            y if y < 1950 => println!("Book published before 1950: {}", book.title),
            y if y < 2000 => println!("Book published in the latter half of the 20th century: {}", book.title),
            _ => println!("Recent book: {}", book.title),
        }
    }
    
    // Demonstrate iterator methods
    let classic_count = library.books.values()
        .filter(|book| book.is_classic())
        .count();
    println!("Number of classics: {}", classic_count);
    
    // Demonstrate closures
    let old_books: Vec<_> = library.books.values()
        .filter(|b| b.year < 1950)
        .map(|b| &b.title)
        .collect();
    println!("Books published before 1950: {:?}", old_books);
    
    // Demonstrate error handling
    let isbn = "9780451524935";
    match library.get_book(isbn) {
        Some(book) => println!("Found book: {}", book.title),
        None => println!("Book with ISBN {} not found", isbn),
    }
    
    // Demonstrate threading with shared state
    let library_arc = Arc::new(Mutex::new(library));
    
    let mut handles = vec![];
    
    for i in 0..3 {
        let library_clone = Arc::clone(&library_arc);
        let handle = thread::spawn(move || {
            let lib = library_clone.lock().unwrap();
            println!("Thread {}: Library has {} books", i, lib.count_books());
            thread::sleep(Duration::from_millis(100));
        });
        handles.push(handle);
    }
    
    for handle in handles {
        handle.join().unwrap();
    }
    
    // Final message
    println!("Library program completed successfully!");
}