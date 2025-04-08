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
