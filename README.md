# Fusion

Vague client-server model, but both client and server will behave the same way.
But with these differences:
Client listens to connection from ssh (like `ssh -N -f -D 9150 localhost:5021`, it would listen on 5021 for this connection, then socks to localhost 9150). Server makes connection to localhost:22
Client makes connection to server, server listens for connection from client.
Client provides the server with the session ID.


Client listens for exactly one (1) connection on what it's listening for because making that assumption makes things easier idk.
Client generates a session ID randomly once it receives a connection. (multiple socks things all get bundled into one big long-lived ssh tunnel, and this will chop it all up and stuff)

At the beginning of a client to server connection, client just gives the session ID then it's off to the races with symmetric communication.

Server reads the session ID, and looks up if it already has a connection for this session ID and adds this one otherwise make a new session. Each session corresponds with one connection from the server to localhost:22.



Other than that, once the sockets are created, they behave the same way. Symmetric behavior for both upload and download.




Most packets will be data packets. Sequence ID, Length, Data.
Once one is received:
	check, what was the most recent sequence ID we forwarded
	if it's the sequence ID of this one, minus one:
		all is good, increment the sequence ID and forward to ssh
		check if we already received a higher sequence number and this just filled in a gap, if so, do that stuff
	else:
		cry. save it for later, and mark this sequence ID as received but not forwarded in a map or something
		if we have a big gap (like if the distance between this and the most recent forwarded is like 10), complain about a misbehaving connection and ask for the missing ones in that range


Upon receiving data from ssh conn:
	ez, just increment sequence id and send as packet on a random connection. (or round robin i dont care [or something even smarter with probability and the relative connection speeds])




