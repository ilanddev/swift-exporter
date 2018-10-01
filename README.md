# Swift Prometheus Exporter

To get the **swift_exporter** plugin install, you will need the following:

1. swift_exporter (the binary, not the .go file)
2. swift_exporter.service 

After getting both files, place them at the following locations:

1. swift_exporter = `/opt/ss/support/bin/`
2. swift_exporter.service = `/usr/lib/systemd/system`

If, for any reason, you need to change the location of where these files are placed, please modify the **swift_exporter.service** file accordingly. 