package main

import (
	"context"
	"fmt"
	influx "github.com/influxdata/influxdb1-client/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"net"
	ping "github.com/go-ping/ping"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
	"time"
)

func resolveHostIp() ([] string) {

	netInterfaceAddresses, err := net.InterfaceAddrs()
	var IPs []string

	if err != nil {
		return IPs
	}

	for _, netInterfaceAddress := range netInterfaceAddresses {

		networkIp, ok := netInterfaceAddress.(*net.IPNet)

		if ok && !networkIp.IP.IsLoopback() && networkIp.IP.To4() != nil {

			ip := networkIp.IP.String()

			log.Println("Resolved Host IP: " + ip)
			IPs = append(IPs, ip)
		}
	}
	return IPs
}

func contains(ips []string, ip string) bool{
	for _,el :=range ips{
		if el == ip {
			return true
		}
	}
	return false
}

func sendPingToAll(Destinations []string, ipSources []string, config influx.BatchPointsConfig) (influx.BatchPoints,error){
	batch,err := influx.NewBatchPoints(config)
	if err != nil{
		log.Fatal("Unable to create batch points instance. Reason: ",err)
		return nil, err
	}
	var strSrc string

	for _,srcIP:=range(ipSources){
		strSrc+=srcIP+","
	}

	for _,ip := range(Destinations){
		pinger, err := ping.NewPinger(ip)
		if err != nil {
			log.Fatal(err)
		}
		pinger.Count = 3
		err = pinger.Run() // Blocks until finished.
		if err != nil {
			log.Fatal(err)
		}
		stats := pinger.Statistics()
		log.Println(stats)
		log.Println(pinger.Source)

		tags := make(map[string]string)
		vals := make(map[string]interface{})
		tags["src"]=strSrc
		vals["sourceIP"] =strSrc
		tags["dst"]=ip
		vals["destinationIP"]=ip
		vals["delay"]=float64(stats.AvgRtt.Milliseconds())

		point,err := influx.NewPoint("delay",tags,vals,time.Now())
		if err !=nil {
			log.Fatal("Unable to create the point")
		}
		batch.AddPoint(point)
	}
	return batch,nil
}

func interval() (int,error) {

	fmt.Println("Provide integer as a number of seconds of interval between measurements." +
		" eg.: 10  will mean each 10 seconds a measurement will take plaace")
	var a int
	_, err := fmt.Scanln(&a)
	if err!=nil{
		fmt.Println("Wrong number provided")
		return 0, err
	}
	return a, nil
}
func ip()(string){

	fmt.Println("Ip address or hostname (dns resolvable) to add temporarily to the ping statistics of this device.")
	var a string
	fmt.Scanln(&a)
	return a
}

func main() {

	f, err := os.OpenFile("flowdaemon.log", os.O_RDWR | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)
	IPs := resolveHostIp()
	if len(IPs)<=0{
		log.Panic("This device doesn't have an IP address.")
	}
	var Destinations []string

	if len(os.Args) < 2{
		log.Panic("No mongodb ip provided")
	}
	if len(os.Args) < 3{
		log.Panic("No influxdb ip provided")
	}
	mongodbIP := os.Args[1]
	influxIP :=os.Args[2]
	mongoURL := "mongodb://"+mongodbIP+":27017"
	client, errMongo := mongo.NewClient(options.Client().ApplyURI(mongoURL))
    if errMongo != nil{
    	log.Panic("Unable to reach the mongodb instance under the url "+mongoURL+". Reason"+errMongo.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	defer func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()
	if err != nil{
		log.Panic("Unable to connect to the mongodb instance under the url "+mongoURL+". Reason"+err.Error())
	}
	err = client.Ping(ctx, readpref.Primary())
	if err != nil{
		log.Panic("Unable to connect to the mongodb instance under the url "+mongoURL+". Reason"+err.Error())
	}
	log.Println("Trying to access the autopolicy database with the flows collection.")
	flows := client.Database("autopolicy").Collection("flows")
	cur, err  :=flows.Find(ctx,bson.D{})
	if err != nil { log.Fatal(err) }
	defer cur.Close(ctx)
	log.Println("Discovered flows: ")
	for cur.Next(ctx) {
		var result bson.M
		err := cur.Decode(&result)
		if err != nil { log.Fatal(err) }
		log.Println("SRC: "+ fmt.Sprint(result["src"])+"   DST: "+fmt.Sprint(result["dst"]))
		if contains(IPs,fmt.Sprint(result["src"])){
			Destinations = append(Destinations, fmt.Sprint(result["dst"]))
		}
	}
	log.Println("Found ",fmt.Sprint(len(Destinations))," my flows. Destinations: ",Destinations)
	log.Println("Connecting to the influxDB on ip : ",influxIP)
	influxConfig := influx.HTTPConfig{
		Addr: "http://"+influxIP+":8086",
		Timeout: time.Duration(15*time.Second),
		InsecureSkipVerify: false}

	clientInflux, err := influx.NewHTTPClient(influxConfig)


	if err!=nil{
		log.Panic("Unable to connect with the InfluxDB. Reason: ",err)
	}
	defer clientInflux.Close()
	influxBPConfig := influx.BatchPointsConfig{Database: "dbping"}

	ticker := time.NewTicker(10 * time.Second)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				batch, err := sendPingToAll(Destinations,IPs,influxBPConfig)
				if err != nil{
					log.Fatal("Error while obtaining the batch points. Reason: ",err)
				}
				err = clientInflux.Write(batch)
				if err != nil{
					log.Fatal("Error while writing the batch points. Reason: ",err)
				}
				log.Println("Written ",len(batch.Points())," to the influxdb")
			}
		}
	}()

	exit :=false
	for{
		fmt.Println("Enter action you want to take. Type end to close the program. " +
			"Type interval to set the interval between measurements. Type destination to" +
			"add another destination (temporary)")
		// var then variable name then variable type
		var command string

		// Taking input from user
		fmt.Scanln(&command)

		switch command{
		case "end":
			{
				done <- true
				exit = true
				break
			}
		case "interval":
			{
				interval,err:= interval()
				if err!=nil{
					break
				}
				ticker = time.NewTicker(time.Duration(interval)*time.Second)
				fmt.Println("Interval set to ",interval," [s]")
				break
			}
		case "destination":
			{
				Destinations = append(Destinations,ip())
				break
			}
		}
		if exit==true{
			break
		}
	}

}