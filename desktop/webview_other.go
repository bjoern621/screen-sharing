//go:build !webkit2_41

package main

// WebView2 (Windows) and WKWebView (macOS) expose RTCPeerConnection without
// configuration; only the WebKitGTK build needs the switch flipped.
func enableWebviewWebRTC() {}
