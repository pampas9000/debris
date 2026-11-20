package main

import (
	"log"

	"debris/cmd"

	"github.com/egoist/mygo"
)

func main() {
	mygo.Bind(NewDebrisService(cmd.NewDefaultEngine()))

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:  "Debris",
			URL:    "/",
			Width:  980,
			Height: 740,
		})
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
