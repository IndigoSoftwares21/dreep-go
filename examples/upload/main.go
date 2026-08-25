// Command upload demonstrates uploading an image to Dreep with the dreep-go
// SDK.
//
// Set DREEP_API_KEY, then run:
//
//	export DREEP_API_KEY=drp_live_...
//	go run ./examples/upload path/to/image.jpg
//
// The file argument defaults to "input.jpg". The upload applies a WebP
// re-encode at 1200px wide and prints the resulting asset plus a delivery-time
// thumbnail URL.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/IndigoSoftwares21/dreep-go"
)

func main() {
	apiKey := os.Getenv("DREEP_API_KEY")
	if apiKey == "" {
		log.Fatal("DREEP_API_KEY is not set")
	}

	path := "input.jpg"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Fatalf("stat %s: %v", path, err)
	}

	client, err := dreep.New(apiKey)
	if err != nil {
		log.Fatalf("creating client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	asset, err := client.Upload(ctx, dreep.UploadOptions{
		File:     f,
		Filename: fi.Name(),
		Destination: dreep.Destination{
			Folder: "examples/go",
		},
		Transform: &dreep.Transform{
			Width:   1200,
			Format:  dreep.FormatWebP,
			Quality: 80,
		},
		KnownSize: fi.Size(),
	})
	if err != nil {
		log.Fatalf("upload failed: %v", err)
	}

	fmt.Println("Uploaded!")
	fmt.Printf("  id:       %s\n", asset.ID)
	fmt.Printf("  url:      %s\n", asset.URL)
	fmt.Printf("  size:     %d bytes\n", int64(asset.SizeBytes))
	if asset.Width > 0 && asset.Height > 0 {
		fmt.Printf("  dimensions: %dx%d\n", asset.Width, asset.Height)
	}

	fmt.Println("\n400px-wide delivery URL:")
	fmt.Println("  " + client.URL(asset.ID, &dreep.Transform{Width: 400}))
}
