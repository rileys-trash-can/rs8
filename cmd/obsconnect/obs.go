package main

import (
	obs "github.com/andreykaipov/goobs"
	obse "github.com/andreykaipov/goobs/api/events"
	obss "github.com/andreykaipov/goobs/api/events/subscriptions"
	obssc "github.com/andreykaipov/goobs/api/requests/scenes"
	obst "github.com/andreykaipov/goobs/api/requests/transitions"
	"github.com/rileys-trash-can/newtecrs82obs"
	"gopkg.in/yaml.v3"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"time"
)

var Config config

type config struct {
	Serial string `yaml:"serial.port"`

	Boolshit bool `yaml:"boolshit"` // enables disables bullshit

	OBSAddr string `yaml:"obs.addr"` // ip with port
	OBSPass string `yaml:"obs.pass"` // password

	TransAuto string `yaml:"trans.auto"` // transition used for auto button
	TransTake string `yaml:"trans.take"` // transition used for take button
}

func readconfig() {
	f, err := os.OpenFile("config.yml", os.O_RDONLY, 0755)
	if err != nil {
		log.Fatalf("Failed to open config: %s", err)
	}

	defer f.Close()

	dec := yaml.NewDecoder(f)
	err = dec.Decode(&Config)
	if err != nil {
		log.Fatalf("Failed to decode config: %s", err)
	}
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)

	readconfig()

	port := Config.Serial
	conn, err := nt8.Open(port)
	if err != nil {
		log.Fatalf("Failed to connect: %s", err)
	}

	// conn.InitBlynk()

	ch := conn.ReadCh()

	obsc, err := obs.New(Config.OBSAddr,
		obs.WithPassword(Config.OBSPass),
		obs.WithEventSubscriptions(obss.Scenes),
	)
	if err != nil {
		log.Fatalf("Failed to connect: %s", err)
	}

	t, err := obsc.Transitions.GetTransitionKindList()
	if err != nil {
		log.Fatalf("Failed to connect gtkl: %s", err)
	}

	for _, name := range t.TransitionKinds {
		log.Printf(" > %s", name)
	}

	sl, err := obsc.Scenes.GetSceneList()
	if err != nil {
		log.Fatalf("Failed to gsl: %s", err)
	}

	sceneindex := make([]string, 8)
	scenenamemap := make(map[string]int)

	lim := lower(len(sl.Scenes), 8)
	for i := 0; i < lim; i++ {
		sceneindex[i] = sl.Scenes[lim-i-1].SceneName
		scenenamemap[sl.Scenes[lim-i-1].SceneName] = i
	}

	for k, v := range sceneindex {
		log.Printf("%d %v", k, v)
	}
	log.Printf("scenemap: %+#v", scenenamemap)

	const (
		TBAR uint8 = iota
	)

	ticker := time.NewTicker(time.Second / 10)
	changed := make(map[uint8]struct{})
	debounce := make(map[uint16]time.Time)
	var tbarbos float64

	deb := func(t uint16) bool {
		now := time.Now()

		last, ok := debounce[t]
		if !ok {
			debounce[t] = now

			return false
		}

		if last.Before(now.Add(-time.Millisecond * 100)) {
			debounce[t] = now

			return false
		}
		return true
	}

	go obsc.Listen(func(l any) {
		switch event := l.(type) {
		case *obse.CurrentPreviewSceneChanged:
			log.Printf("current preview scene: %s %d", event.SceneName, scenenamemap[event.SceneName]+1)
			program := make([]nt8.CmdLight, 0)

			for i := uint8(0); i < 8; i++ {
				program = append(program, nt8.CmdLight{
					Type:  nt8.ButtonPreview,
					Value: i,
					State: nt8.LightOff,
				})
			}

			program[7-scenenamemap[event.SceneName]].State = nt8.LightOn

			conn.LightCmdCh <- program
			break

		case *obse.CurrentProgramSceneChanged:
			log.Printf("current program scene: %s %d", event.SceneName, scenenamemap[event.SceneName]+1)
			program := make([]nt8.CmdLight, 0)

			for i := uint8(0); i < 8; i++ {
				program = append(program, nt8.CmdLight{
					Type:  nt8.ButtonProgram,
					Value: i,
					State: nt8.LightOff,
				})
			}

			program[7-scenenamemap[event.SceneName]].State = nt8.LightOn

			conn.LightCmdCh <- program
			break

		default:
			//log.Printf("non handled thingy %T", event)
			break
		}
	})

	for {
		select {
		case <-ticker.C:
			if _, ok := changed[TBAR]; ok {
				_, err = obsc.Transitions.SetTBarPosition(&obst.SetTBarPositionParams{
					Position: i(tbarbos),
					Release:  i(tbarbos == 0 || tbarbos == 1),
				})
				if err != nil {
					log.Printf("Error setting tbar: %s", err)
				}

			}

			changed = make(map[uint8]struct{})

		case e := <-ch:
			switch event := e.(type) {
			case *nt8.EventButton:
				if event.Direction != 0 {
					continue
				}

				switch event.Type {
				case nt8.ButtonProgram:
					log.Printf("Setting Program to %d", 7-event.Value)
					obsc.Scenes.SetCurrentProgramScene(&obssc.SetCurrentProgramSceneParams{
						SceneName: &sceneindex[7-event.Value],
					})

					break
				case nt8.ButtonPreview:
					log.Printf("Setting Program to %d", 7-event.Value)
					obsc.Scenes.SetCurrentPreviewScene(&obssc.SetCurrentPreviewSceneParams{
						SceneName: &sceneindex[7-event.Value],
					})

					break
				case nt8.ButtonAutoTakeDSK:
					switch event.Value {
					case 0x01: // TAKE or 0x02
						if deb(nt8.ButtonAutoTakeDSK<<8 | 0x01) {
							continue
						}

						go func() {
							oldprog, err := obsc.Scenes.GetCurrentProgramScene()
							if err != nil {
								log.Printf("oldprogram: %s (uuid %s)", oldprog.SceneName, oldprog.SceneUuid)
							}

							_, err = obsc.Transitions.SetCurrentSceneTransition(&obst.SetCurrentSceneTransitionParams{
								TransitionName: &Config.TransTake,
							})
							if err != nil {
								log.Printf("set transition: %s", err)
							}

							_, err = obsc.Transitions.TriggerStudioModeTransition()
							if err != nil {
								log.Printf("Studiomodetransition: %s", err)
							}

							_, err = obsc.Scenes.SetCurrentPreviewScene(&obssc.SetCurrentPreviewSceneParams{
								SceneUuid: &oldprog.SceneUuid,
							})
							if err != nil {
								log.Printf("set current preview: %s", err)
							}
						}()
					case 0x03: // AUTO

						go func() {
							oldprog, err := obsc.Scenes.GetCurrentProgramScene()
							if err != nil {
								log.Printf("oldprogram: %s (uuid %s)", oldprog.SceneName, oldprog.SceneUuid)
							}

							_, err = obsc.Transitions.SetCurrentSceneTransition(&obst.SetCurrentSceneTransitionParams{
								TransitionName: &Config.TransAuto,
							})
							if err != nil {
								log.Printf("set transition: %s", err)
							}

							_, err = obsc.Transitions.TriggerStudioModeTransition()
							if err != nil {
								log.Printf("Studiomodetransition: %s", err)
							}

							_, err = obsc.Scenes.SetCurrentPreviewScene(&obssc.SetCurrentPreviewSceneParams{
								SceneUuid: &oldprog.SceneUuid,
							})
							if err != nil {
								log.Printf("set current preview: %s", err)
							}
						}()
						if Config.Boolshit {
							go exec.Command("gti").Run()
						}

					case 0x05: // DDR
						if Config.Boolshit {
							println(" ____  ____  ____\n|  _ \\|  _ \\|  _ \\\n| | | | | | | |_) |\n| |_| | |_| |  _ <\n|____/|____/|_| \\_\\")
							println(ddrquote())
						}
					}
				}
				break

			case *nt8.EventSlider:
				if event.Type == nt8.SliderTbar {
					value := (float64(event.Value)) / 250
					if value > 1 {
						value = 1

					}

					if value < 0 {
						value = 0
					}

					log.Printf("tbar %1.3f", value)

					tbarbos = value
					changed[TBAR] = struct{}{}
					continue
				}

				log.Printf("%X %3d", event.Type, event.Value)

			}
		}
	}

	obsc.Disconnect()
}

func i[K any](a K) *K {
	return &a
}

func lower(a, b int) int {
	if a > b {
		return b
	}

	return a
}

var ddrquotes = []string{
	"Niemand hat die Absicht eine Mauer zu bauen",
	"Den Sozialismus in seinem Lauf, hält weder Ochs noch Esel auf.",
	"Frieden ist nicht alles, aber ohne Frieden ist alles nichts.",
	"Wir müssen lernen, wie die Gesellschaft so zu gestalten, dass die Menschen glücklich sind.",
	"Die Partei hat immer recht.",
	"Vorwärts immer, rückwärts nimmer!",
	"Den Sozialismus in seinem Lauf, hält weder Ochs noch Esel auf.",
	"Frieden ist nicht alles, aber ohne Frieden ist alles nichts.",
	"Wir müssen lernen, wie die Gesellschaft so zu gestalten, dass die Menschen glücklich sind.",
	"Die Partei hat immer recht.",
	"Vorwärts immer, rückwärts nimmer!",
	"Wer kämpft, kann verlieren. Wer nicht kämpft, hat schon verloren.",
	"Es muss demokratisiert werden im tiefsten Sinne des Wortes, die Wirtschaft, die Wissenschaft, das kulturelle Leben.",
	"Freundschaft, das ist das schönste auf der Welt.",
	"Niemand hat die Absicht, eine Mauer zu errichten.",
	"Die Wahrheit ist, dass wir noch nicht da sind, wo wir sein sollten.",
	"Die Zukunft gehört dem Sozialismus.",
	"Der Weg der sozialistischen Partei ist der Weg des Volkes.",
	"Wir haben das Glück, in einer Zeit zu leben, in der wir Zeugen eines großen historischen Wandels sind.",
	"Sozialismus ist das Gegenteil von Egoismus.",
	"Die Kunst ist eine Waffe der Revolution.",
}

func ddrquote() string {
	return ddrquotes[rand.Intn(len(ddrquotes))]
}
