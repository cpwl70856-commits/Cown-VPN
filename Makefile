APP       := mywgclient
SERVER    := mywgserver
KEYGEN    := mywgkeygen
MODULE    := github.com/user/mywgclient
ICON      := assets/Icon.png
FYNE      ?= fyne

.PHONY: all tidy build client server keygen mobile android-aar test run package-linux package-darwin package-windows clean

all: tidy build

tidy:
	go mod tidy

build: client server keygen

client:
	go build -o bin/$(APP) ./cmd/client

server:
	go build -o bin/$(SERVER) ./cmd/server

keygen:
	go build -o bin/$(KEYGEN) ./cmd/keygen

# Compile-check the gomobile package (actual XCFramework / AAR needs gomobile).
mobile:
	go build ./pkg/wgmobile

# Optional gomobile AAR. The Android app uses the Maven tunnel artifact and
# does not need this file. See android/README.md.
android-aar:
	./android/scripts/build_aar.sh

test:
	go test ./...

run: client
	./bin/$(APP)

# Packaging (requires: go install fyne.io/fyne/v2/cmd/fyne@latest)
# Provide assets/Icon.png before packaging for a branded icon.
package-linux:
	$(FYNE) package -os linux -name $(APP) -appID com.user.mywgclient -icon $(ICON) -release ./cmd/client

package-darwin:
	$(FYNE) package -os darwin -name $(APP) -appID com.user.mywgclient -icon $(ICON) -release ./cmd/client

package-windows:
	$(FYNE) package -os windows -name $(APP) -appID com.user.mywgclient -icon $(ICON) -release ./cmd/client

clean:
	rm -rf bin/ $(APP) $(APP).app $(APP).exe $(APP).tar.xz
