// JavaScript Example

// Class definition
class User {
    constructor(name, email) {
        this.name = name;
        this.email = email;
        this.createdAt = new Date();
    }
    
    getInfo() {
        return `${this.name} (${this.email})`;
    }
    
    static validateEmail(email) {
        return /^\w+([\.-]?\w+)*@\w+([\.-]?\w+)*(\.\w{2,3})+$/.test(email);
    }
}

// Async function example
async function fetchUserData(userId) {
    try {
        const response = await fetch(`https://api.example.com/users/${userId}`);
        if (!response.ok) {
            throw new Error('Network response was not ok');
        }
        return await response.json();
    } catch (error) {
        console.error('Error fetching user data:', error);
        return null;
    }
}

// Array methods
const numbers = [1, 2, 3, 4, 5];
const doubled = numbers.map(num => num * 2);
const evens = numbers.filter(num => num % 2 === 0);
const sum = numbers.reduce((total, num) => total + num, 0);

// Template literal
const greeting = `Hello ${new User('John', 'john@example.com').name}!
Welcome to our application.
Current time: ${new Date().toLocaleTimeString()}`;

// Event listener
document.getElementById('myButton').addEventListener('click', function() {
    console.log('Button clicked!');
    const user = new User('Guest', 'guest@example.com');
    document.getElementById('userInfo').textContent = user.getInfo();
});

// Export
export { User, fetchUserData };
