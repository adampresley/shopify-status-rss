# About

This is a small server that scrapes the Shopify Status page periodically and provides an RSS feed for when the various Shopify services are not "Operational".

# Architecture

This application is written using:

- Go 1.26
- Uses github.com/adampresley/mux as a thin wrapper around The Go standard library mux and routing 
- Uses github.com/adampresley/httphelpers to provide generic methods to retrieve request data and respond
- Uses GORM (https://gorm.io) for database ORM to support SQLite and Postgres

# Rules

- After creating or modifying a Go file, always run `goimports -w <changed file or package>`, where **changed file or package** is the file or package for files where you made a change
- In Go functions, **always** declare variables at the top of the function in an `var ()` block. If the function needs an error variable, the very first variable decalred in the block must be `err error`. 
- All file names must be lower-hypen case.
- This application uses GORM (> 1.30) for ORM and database interactions. This version supports generics. You must use the generics interface. See https://gorm.io/docs/ for more information. 
