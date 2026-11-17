// Package appcontainer is what Sona's Windows sandboxes are made of: an
// AppContainer profile (a package SID with no capabilities but those given
// it), the access grants that let that SID reach the folders it needs, and a
// process started in the AppContainer inside a job object. The playback host
// (internal/playback/cdmhost) and the add-ons (internal/addons/sandbox) use
// it; each decides what its own sandbox is granted.
//
// Windows checks every access of an AppContainer process twice, against the
// user and against the package SID (and ALL APPLICATION PACKAGES), so the
// process reaches only what the SID has been granted, and the network stack
// refuses all its traffic, the loopback included, unless it holds the
// capabilities for it.
//
// The decisions about access lists are plain Go on plain values (acl.go), so
// that Linux runs their tests; the rest is Windows only.
package appcontainer
