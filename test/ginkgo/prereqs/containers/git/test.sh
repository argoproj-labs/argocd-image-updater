#!/bin/bash

set -o errexit
set -o xtrace

main() {
  init_docker_container
  sleep 3
  assert_can_clone
}

init_docker_container() {
  docker-compose \
    -f ./example/docker-compose.yml \
    up \
    -d
}

assert_can_clone() {
  git clone http://localhost:8080/myrepo.git
  [[ -f "myrepo/myfile.txt" ]] || exit 1

  echo "OK!"
}

cleanup() {
  local exit_code=$?

  echo "Exited with [$exit_code]"
  docker-compose \
    -f ./example/docker-compose.yml \
    stop
  rm -rf ./myrepo
}

trap cleanup EXIT
main
