# Swift Prometheus Exporter

To get the **swift_exporter** plugin install, you will need the following:

1. swift_exporter (the binary, not the .go file)
2. swift_exporter.service 

To generate the swift_exporter binary, do the following:
1. navigate to the root of this project
2. run `go get` to retrieve project dependencies
3. run `go build -o swift_exporter` to generate the executable in the project folder

After getting both files, place them at the following locations:

1. swift_exporter = `/opt/ss/support/bin/`
2. swift_exporter.service = `/usr/lib/systemd/system`

The path for the `swift_exporter` binary assumes an installation on a SwiftStack Swift node. If you're not running SwiftStack, you can modify the path to `/usr/local/bin/` or any other location you prefer. If you do, please also remember to modify the **swift_exporter.service** file accordingly. 