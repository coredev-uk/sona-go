#!/bin/sh
# A protocol v2 add-on in plain shell, the conformance client for
# internal/cli's tests: it says hello, subscribes to now-playing, receives
# the event, and is refused a command it has no permission for. Standard
# input and output are the channel; the log goes to standard error.
set -eu

say() { printf '%s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

say '{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["playback.read","playback.control"]}}'
read -r line || fail "no answer to hello"
case $line in
  '{"id":1,"result":{"id":"demo","protocol":"2.0.0","permissions":["playback.read"]'*) ;;
  *) fail "hello answered: $line" ;;
esac

say '{"id":2,"method":"subscribe","params":{"topics":["now-playing"]}}'
reply=0
event=0
while [ "$reply" -eq 0 ] || [ "$event" -eq 0 ]; do
  read -r line || fail "the channel closed before the event"
  case $line in
    '{"id":2,"result":{"topics":["now-playing"]}}') reply=1 ;;
    '{"event":"now-playing","data":{"track":{"id":"100","title":"Come Together"'*) event=1 ;;
    *) fail "unexpected frame: $line" ;;
  esac
done

# The add-on asked for playback.control, but the user granted only
# playback.read: the command is refused.
say '{"id":3,"method":"call","params":{"command":"play"}}'
read -r line || fail "no answer to play"
case $line in
  '{"id":3,"error":{"code":"permission_denied"'*) ;;
  *) fail "play answered: $line" ;;
esac
