# Drone Commander Tello Driver

Applicazione desktop nativa in Go che carica i programmi XML esportati da Drone Commander e li esegue su un Ryze/DJI Tello tramite Tello SDK 2.0. La GUI usa Fyne: si apre come una normale finestra desktop e non avvia un server HTTP o un browser.

## Unita' indoor

La conversione e' fissa:

```text
1 unita Drone Commander = 1 centimetro
```

Il Tello accetta i movimenti lineari `up`, `down`, `left`, `right`, `forward` e `back` soltanto da 20 a 500 cm. Un blocco `Cammina 1`, quindi, viene fermato con un errore; per avanzare di 30 cm bisogna usare `Cammina 30`.

Il blocco `Ritorna alla base` torna alle coordinate X/Z iniziali mantenendo la quota corrente. Questa scelta evita di riprodurre indoor la vecchia quota simulata di 10 unita', che ora significherebbe appena 10 cm.

Durante il volo il driver rifiuta inoltre quote programmate inferiori a 20 cm; per scendere sotto tale soglia bisogna usare il blocco `Atterra`.

## Avvio

Requisiti per compilare: Go 1.22, un compilatore C e le librerie grafiche richieste da Fyne.

Su Debian/Ubuntu:

```sh
sudo apt-get install gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev
git clone git@github.com:vroby65/DroneCommander-Driver.git
cd DroneCommander-Driver
go run .
```

Dalla finestra:

1. scegli un file `.xml` salvato da Drone Commander;
2. attiva **Modalita simulazione**, connettiti e prova il programma;
3. per il volo reale collega il computer alla rete Wi-Fi `TELLO-...`, disattiva la simulazione e connettiti;
4. verifica la soglia batteria e avvia il programma.

Il file [examples/quadrato.xml](examples/quadrato.xml) contiene un percorso indoor di 50 cm per lato da provare prima in simulazione.

## Compilazione

Piattaforma corrente:

```sh
make build
```

Linux AMD64:

```sh
make build-linux
```

Windows AMD64 da Linux richiede MinGW-w64:

```sh
make build-windows
```

macOS richiede una toolchain Apple compatibile. Da Linux il target usa Docker, l'immagine di `fyne-cross` e un SDK macOS estratto da Xcode/Command Line Tools:

```sh
make build-macos MACOS_SDK_PATH=/percorso/MacOSX.sdk
```

Fyne usa CGO per accedere alle API grafiche native. Per questo la semplice impostazione di `GOOS` non e' sufficiente: ogni target necessita del relativo compilatore C e dei rispettivi header/SDK. Su un Mac e' sufficiente usare `make build` per l'architettura locale.

## Conversione dei comandi

| Blocco Drone Commander | Comando/comportamento Tello |
|---|---|
| Decolla / Atterra | `takeoff` / `land` |
| Cammina | `forward` o `back`, valore in cm |
| Scivola | `right` o `left`, valore in cm |
| Cambia quota | `up` o `down`, valore in cm |
| Vai a / Muovi di | rotazione e/o `go x y z speed` |
| Curva / Curva assoluta | `curve x1 y1 z1 x2 y2 z2 speed` |
| Imposta/cambia angolo | `cw` o `ccw` |
| Velocita | centimetri al secondo, limitati a 10-100 |
| Attendi | attesa locale, con keep-alive ogni 8 secondi |
| Fumo | ignorato con una nota nel registro |

Il driver mantiene una posizione stimata per tradurre i movimenti assoluti. Il Tello standard non fornisce una posizione globale indoor senza riferimenti esterni: vento, urti e deriva possono rendere imprecisi `Vai a`, `Curva assoluta` e `Ritorna alla base`.

## Sicurezza

- Provare ogni programma in simulazione.
- Usare paraeliche e liberare completamente l'area indoor.
- Impostare quote e distanze in centimetri, ricordando il minimo SDK di 20 cm per comando lineare.
- Lasciare attivo l'atterraggio automatico finale.
- **Stop / hovering** annulla il programma e invia `stop`.
- **Atterra** annulla il programma e invia `land`.
- **Arresto motori** invia `emergency`: il drone cade immediatamente.

L'esecuzione e' limitata a 10.000 blocchi per interrompere cicli non terminanti. I blocchi non supportati vengono segnalati prima del volo.

## Opzioni

```text
-tello 192.168.10.1:8889     indirizzo UDP comandi
-state :8890                 bind locale per la telemetria
```

## Struttura

- `desktopui/` — finestra nativa Fyne e selettore file;
- `session/` — stato applicativo, connessione ed esecuzione;
- `program/` — parser XML Blockly e interprete;
- `flight/` — conversione dei blocchi in comandi Tello;
- `tello/` — protocollo UDP, telemetria e simulatore offline.

## Test

```sh
go test ./...
```
