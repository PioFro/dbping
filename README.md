# DBPING

DBPING reads flows to measure from the mongoDB instance. The flow format in the database must be as follows:
```json
{
    "src" : "192.168.0.2",
    "dst" : "192.168.0.1"
}
```
This tells the daemon on running on the source machine (192.168.0.2) that it should start gathering and uploading to the InfluxDB database measurement on the 
flow leading to the destination machine. It gathers N ping RTT and uploads the average of N RTTs. 
For now N=3 and cannot be changed it will be changed in next commits.

In the go.mod file all required modules are listed.

There are two required commandline arguments - the order of the arguments is important.

* First argument is the IP address of the mongoDB instance where the autopolicy database with flows collection that stores flows as described higher is kept.
* Second argument is the IP address of the influxDB instance in which obtained results will be put. Note that the data stored by the Daemons is identified by the IP addresses of that deamons.

To run the compiled program use:

```bash
./go_build_flowdaemon_go localhost localhost
```
since the go-lang will compile the program to a single executable file.
