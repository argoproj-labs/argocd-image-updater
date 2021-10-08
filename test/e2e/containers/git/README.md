# gitserver-http [![Build Status](https://travis-ci.org/cirocosta/gitserver-http.svg?branch=master)](https://travis-ci.org/cirocosta/gitserver-http)

> A git server with Nginx as the HTTP frontend and fast cgi wrapper for running the git http backend


## Usage

To run a git server without any repositories configured in advance but allowing them to be saved into `./repositories`: 
 
  ```sh
  docker run \
    -d  \                                 # deamonize
    -v `pwd`/repositories:/var/lib/git \  # mount the volume
    -p "8080:80" \                        # expose the port 
    cirocosta/gitserver-http
  ```

Now, initialize a bare repository:

  ```sh
  cd repositories
  git init --bare myrepo.git
  ```

and then, just clone it somewhere else:

  ```sh
  cd /tmp
  git clone http://localhost:8080/myrepo.git
  cd myrepo 
  ```


### Pre-Initialization

Git servers work with bare repositories. This image provides the utility of initializing some pre-configured repositories in advance. Just add them to `/var/lib/initial` and then run the container. For instance, having the tree:

  ```
  .
  └── initial
      └── initial
          └── repo1
              └── file.txt
  ```

and then executing

  ```sh
  docker run \
    -d  \                                 # deamonize
    -v `pwd`/initial:/var/lib/initial \   # mount the initial volume
    -p "8080:80" \                        # expose the port 
    cirocosta/gitserver-http              # start git server and init repositories
  ```

will allow you to skip the `git init --bare` step and start with the repositories pre-"installed" there:

  ```sh
  git clone http://localhost/repo1.git
  cd repo1 && ls
  # file.txt
  ```


## Example

to run the example:

  ```sh
  make example
  ```


This will create a git server http service on `:80`. Now you can clone the sample repository:


  ```sh
  git clone http://localhost:8080/repo1.git
  ```


