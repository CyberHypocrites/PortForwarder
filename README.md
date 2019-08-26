# Port Forwarder
A small program to port forward with quota options
## What is this?
This a small program to forward ports with a quota option to control the data users use.
Some of the features of this code are:
* **Lightweight**: It has no dependencies, just the main file, and standard library.
* **Easy to use**: Just edit the rules file and you can use the proxy
* **High performance**: With iperf3 I achieved 2.86 Gbits/sec in a local tunnel.
* **Simultaneous Connections Limit**: Limit the amount of simultaneous _connections_ that a port can have.
* **Soft Blocking**: Block the new incoming connections and keep the old ones alive when the quota reaches. [Read More](#Soft-Blocking)

## Install
Just head to [releases](https://github.com/HirbodBehnam/PortForwarder/releases) and download one for your os.
### Build from source
Building this is not that hard. At first install [golang](https://golang.org/dl/) for your operating system. Clone this repository and run this command:
```bash
go build main.go
```
The executable file will be available at the present directory.
## How to use it?
Did you download the executable for your os? Good!

Edit the `rules.json` file as you wish. Here is the cheatsheet for it:
* `SaveDuration`: The program writes the quotas to disk every `SaveDuration` seconds. Default is 600 seconds or 10 minutes.
* `Timeout`: The time in seconds that a connection can stay alive without transmitting any data. Default is ten minutes.
* `TimeoutCheck`: The time in seconds to check if connections are still alive. Default is 60 seconds.
* `Rules` Array: Each element represents a forwarding rule and quota for it.
    * `Listen`: The local port to accept the incoming connections for proxy.
    * `Forward`: The address that the traffic must be forwarded to. Enter it like `ip:port`
    * `Quota`: The number of bytes the user can transfer.
    * `Simultaneous`: Amount of allowed simultaneous connections to this port. Use 0, or remove it for unlimited.
    
Save the file and just open the main executable to run the proxy.
### Unlimited Quota
Well, you can't but actually you can!

The max quota value is `92233720368547758087`. You can use this.
### Arguments
There are two options:
1. `-h`: It prints out the help of the proxy.
2. `-v`: Verbose mode
3. `--config`: In case you want to use a config file with another name, just pass it to program as the first argument. For example:
```bash
./PortForwarder --config custom_conf.json
```
## How it works?
The base code is [this](https://gist.github.com/qhwa/cb9d3851450bff3b705e)(Thanks man!). The code is changed in order to measure the traffic transmitted.
### Soft Blocking
So here is a part you should read. The proxy uses the `io.Copy` function([reference](https://golang.org/pkg/io/#Copy)) in order to write the buffers. The good point is that the buffer is not with me and it is with the golang itself and there is no loop. But there is a catch: This function returns when it reaches the EOF or in case of an error.

So what's wrong with this? Well, I can understand how many bytes had been transferred when the function returns. So here comes soft connections and fast forward in cost of inaccuracy.

As soon as the function returns, I the quota will change.

And what do I mean about the softer connections? The client can use the program after the quota is reached. When the client wants to establish a new connection it will be rejected from the server. Plus you can manage how much client has used more than its quota.

### Timeout
I implemented a custom timeout method for this. It is dead simple:

Save the last time each _connection_ has transmitted something. (On each write or read function). Then every minute(default), the app reads all of the last transmit time. If it is larger than (now - timeout), close the connection.

## Other Stuff
[Persian guild to setup this with mtproto](http://www.mediafire.com/file/4u3khp5oj7ecgxk/%25D9%2585%25D8%25AD%25D8%25AF%25D9%2588%25D8%25AF_%25DA%25A9%25D8%25B1%25D8%25AF%25D9%2586_%25DA%25A9%25D8%25A7%25D8%25B1%25D8%25A8%25D8%25B1%25D8%25A7%25D9%2586.pdf/file)
