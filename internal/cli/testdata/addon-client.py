#!/usr/bin/env python3
"""A protocol v2 add-on in Python, the conformance client for internal/cli's
tests: hello, subscribe to now-playing, receive the event, and be refused a
command it has no permission for. Standard input and output are the
channel; the log goes to standard error."""
import json
import sys


def send(frame):
    sys.stdout.write(json.dumps(frame) + "\n")
    sys.stdout.flush()


def recv():
    line = sys.stdin.readline()
    if not line:
        sys.exit("FAIL: the channel closed")
    return json.loads(line)


def answer(frame_id):
    """The response to frame_id; events met on the way are returned too."""
    events = []
    while True:
        frame = recv()
        if frame.get("id") == frame_id and "method" not in frame:
            return frame, events
        if "event" in frame:
            events.append(frame)
            continue
        sys.exit("FAIL: unexpected frame %r" % (frame,))


send({"id": 1, "method": "hello", "params": {
    "id": "demo", "protocol": {"min": "2.0.0", "max": "2.0.0"},
    "permissions": ["playback.read", "playback.control"]}})
hello, _ = answer(1)
if hello["result"]["protocol"] != "2.0.0" or hello["result"]["permissions"] != ["playback.read"]:
    sys.exit("FAIL: hello answered %r" % (hello,))

send({"id": 2, "method": "subscribe", "params": {"topics": ["now-playing"]}})
sub, events = answer(2)
if sub["result"]["topics"] != ["now-playing"]:
    sys.exit("FAIL: subscribe answered %r" % (sub,))
if not events:
    events.append(recv())
event = events[0]
if event["event"] != "now-playing" or event["data"]["track"]["title"] != "Come Together":
    sys.exit("FAIL: event %r" % (event,))

# Asked for playback.control, granted only playback.read: refused.
send({"id": 3, "method": "call", "params": {"command": "play"}})
refused, _ = answer(3)
if refused.get("error", {}).get("code") != "permission_denied":
    sys.exit("FAIL: play answered %r" % (refused,))
