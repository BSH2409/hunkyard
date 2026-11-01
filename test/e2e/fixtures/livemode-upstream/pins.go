package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strings"
	"sync/atomic"
)

// payLabel is the "Pay now" button's text; /modern/toggle flips it so a spec
// can change a pinned element between rounds.
var payLabel atomic.Value

func init() { payLabel.Store("Pay now") }

// registerPins serves the pages the pin-screenshot specs capture: modern CSS
// colour functions on a tall page with the target near the bottom, an app
// that scrolls inside a container, and a cross-origin image.
func registerPins(mux *http.ServeMux) {
	mux.HandleFunc("/modern", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// The image comes from the other loopback name, so it is cross-origin
		// to the page and served without CORS headers.
		other := strings.Replace(host, "127.0.0.1", "localhost", 1)
		if other == host {
			other = strings.Replace(host, "localhost", "127.0.0.1", 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, modernHTML, other, payLabel.Load().(string))
	})
	mux.HandleFunc("/modern/toggle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		label := r.URL.Query().Get("label")
		if label == "" {
			label = "Pay now"
		}
		payLabel.Store(label)
		fmt.Fprint(w, label)
	})
	mux.HandleFunc("/scroll-app", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, scrollAppHTML)
	})
	mux.HandleFunc("/body-scroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, bodyScrollHTML)
	})
	mux.HandleFunc("/covered", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, coveredHTML)
	})
	mux.HandleFunc("/late", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, lateHTML)
	})
	mux.HandleFunc("/pixel.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(bluePixelPNG)
	})
}

// bluePixelPNG is an 8×8 blue PNG.
var bluePixelPNG = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{B: 255, A: 255}}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}()

const modernHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>modern</title>
<style>
  body { margin: 0; font: 16px system-ui, sans-serif; background: oklch(97%% 0.01 250); color: oklch(25%% 0.02 250); }
  header { padding: 24px; background: color(display-p3 0.2 0.4 0.9); color: white; }
  .card { margin: 24px; padding: 24px; border-radius: 12px; background: color-mix(in oklch, white 70%%, oklch(70%% 0.15 150)); }
  .spacer { height: 1900px; }
  #pay { padding: 12px 24px; font-size: 18px; border: 0; border-radius: 8px; background: oklch(55%% 0.2 260); color: white; }
</style></head>
<body>
<header><h1 id="modern-title">Checkout</h1></header>
<section class="card"><p>Order summary</p><img id="remote-img" src="http://%s/pixel.png" width="40" height="40" alt="remote"></section>
<div class="spacer" aria-hidden="true"></div>
<section class="card" id="pay-card"><p>Total $10.</p><button id="pay">%s</button><p>Terms apply.</p></section>
</body>
</html>`

const scrollAppHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>scroll app</title>
<style>
  html, body { margin: 0; height: 100%; overflow: hidden; font: 16px system-ui, sans-serif; }
  #app { height: 100%; overflow-y: auto; background: #f4f6fb; }
  .row { margin: 12px; padding: 16px; background: #dfe7f7; border-radius: 6px; }
  #deep { margin: 12px; padding: 16px; background: #2f6fde; color: white; }
</style></head>
<body>
<div id="app">
  <div class="row">Row 1</div><div class="row">Row 2</div><div class="row">Row 3</div>
  <div style="height: 1800px" aria-hidden="true"></div>
  <button id="deep">Deep action</button>
</div>
</body>
</html>`

// lateHTML builds its content after load, like a single-page app fetching
// data: the pinned button does not exist when the page first loads.
const lateHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>late</title></head>
<body>
<main id="late-main"><h1>Loading…</h1></main>
<script>
setTimeout(function () {
  var main = document.getElementById('late-main');
  main.innerHTML = '<div style="height:300px"></div><button id="late-btn" style="margin-left:200px">Late button</button>';
}, 700);
</script>
</body>
</html>`

// bodyScrollHTML scrolls its body instead of the window, and the body is
// offset from the top, as some app layouts do.
const bodyScrollHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>body scroll</title>
<style>
  html { height: 100%; overflow: hidden; }
  body { position: relative; top: 40px; height: calc(100% - 40px); margin: 0; overflow-y: auto; font: 16px system-ui, sans-serif; }
  #tall { height: 1400px; }
  #body-target { margin-left: 160px; }
</style></head>
<body>
<div id="tall" aria-hidden="true"></div>
<button id="body-target">Body target</button>
<div style="height:600px" aria-hidden="true"></div>
</body>
</html>`

// coveredHTML opens a panel over the pinned button by toggling an attribute,
// as notification drawers do.
const coveredHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>covered</title>
<style>
  body { margin: 0; font: 16px system-ui, sans-serif; }
  #toggle-panel { margin: 10px; }
  #covered-target { position: absolute; top: 120px; left: 120px; }
  #drawer { position: fixed; top: 60px; left: 60px; width: 400px; height: 300px; z-index: 50; background: #fff; border: 1px solid #ccc; }
</style></head>
<body>
<button id="toggle-panel" onclick="var d=document.getElementById('drawer'); d.hidden=!d.hidden;">Notifications</button>
<button id="covered-target">Covered target</button>
<div id="drawer" hidden>Notifications drawer</div>
</body>
</html>`
