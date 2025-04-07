#!/usr/bin/env python3
"""
Example Python file with various language features
"""

import os
import sys
import json
from typing import List, Dict, Optional, Tuple
from dataclasses import dataclass
from collections import defaultdict

# Decorators
def timer(func):
    """Simple timing decorator"""
    def wrapper(*args, **kwargs):
        import time
        start = time.time()
        result = func(*args, **kwargs)
        end = time.time()
        print(f"{func.__name__} took {end - start:.2f} seconds to run")
        return result
    return wrapper

# Type annotations and dataclasses
@dataclass
class Product:
    """Product class for storing product information"""
    id: int
    name: str
    price: float
    tags: List[str] = None
    in_stock: bool = True
    
    def __post_init__(self):
        if self.tags is None:
            self.tags = []
    
    def apply_discount(self, percentage: float) -> float:
        """Apply a discount to the product price"""
        return self.price * (1 - percentage / 100)

# Context manager
class FileManager:
    def __init__(self, filename: str, mode: str = 'r'):
        self.filename = filename
        self.mode = mode
        self.file = None
        
    def __enter__(self):
        self.file = open(self.filename, self.mode)
        return self.file
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        if self.file:
            self.file.close()

# Class inheritance
class ShoppingCart:
    def __init__(self):
        self.items: List[Product] = []
    
    def add_item(self, product: Product, quantity: int = 1):
        for _ in range(quantity):
            self.items.append(product)
    
    def get_total(self) -> float:
        return sum(item.price for item in self.items)

# Generator function
def fibonacci(n: int) -> List[int]:
    """Generate Fibonacci sequence up to n"""
    a, b = 0, 1
    result = []
    for _ in range(n):
        result.append(a)
        a, b = b, a + b
    return result

@timer
def main():
    # List comprehension
    squares = [x**2 for x in range(10) if x % 2 == 0]
    print(f"Squares of even numbers: {squares}")
    
    # Dictionary comprehension
    word_lengths = {word: len(word) for word in ['hello', 'world', 'python']}
    
    # Lambda functions
    sort_by_length = lambda items: sorted(items, key=len)
    words = ["python", "programming", "is", "fun"]
    print(f"Sorted by length: {sort_by_length(words)}")
    
    # Working with files
    try:
        with FileManager('example.txt', 'w') as f:
            f.write('Hello, Python!')
    except IOError as e:
        print(f"I/O error: {e}")
    
    # Error handling
    try:
        value = int(input("Enter a number: "))
        print(f"You entered: {value}")
    except ValueError:
        print("That's not a valid number!")
    finally:
        print("Input processing complete")
    
    # Creating and using classes
    products = [
        Product(1, "Laptop", 999.99, ["electronics", "computers"]),
        Product(2, "Headphones", 149.99, ["electronics", "audio"]),
        Product(3, "Mouse", 29.99)
    ]
    
    # Using defaultdict
    product_by_tag = defaultdict(list)
    for product in products:
        for tag in product.tags:
            product_by_tag[tag].append(product)
    
    print(f"Number of electronic products: {len(product_by_tag['electronics'])}")
    
    # Using generator
    fib_sequence = fibonacci(10)
    print(f"Fibonacci sequence: {fib_sequence}")

if __name__ == "__main__":
    main()
