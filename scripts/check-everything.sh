#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

RED=1
GREEN=2
print_message() {
  message=$1
  colour=$2
  printf "\\r\\033[00;3%sm[%s]\\033[0m\\n" "$colour" "$message"
}

main() {
  print_message "about to run tests in parallel, it will be awesome" $GREEN
  print_message "ctrl-d panes when they are done" $RED
  tmux new-window -n korifi-tests "/bin/bash -c \"make lint; bash --init-file <(echo 'history -s make lint')\""
  tmux split-window -h -p 66 "/bin/bash -c \"make test-controllers-api; bash --init-file <(echo 'history -s make test-controllers-api')\""
  tmux split-window -h -p 50 "/bin/bash -c \"make test-e2e; bash --init-file <(echo 'history -s make test-e2e')\""
}

main
