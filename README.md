# gostream


Go stream is a live streaming serveur that can serve VOIP and group calls

It can function in two modes with a side website that can provide and check identification token linked to user session or in standalone mode using asymetric cryptography

# With side site


The site definition is on the first line of hello.go

`var mysite site = site{siteURL: "http://localhost", siteOrigin: "http://localhost", enable: true}`

siteOrigin is the domain of the site that host the page that will use the streaming api with CORS requests

siteURL is the base URL for the site API 

* /Membres/newCRSF => return a token to be used in subsequent request based on user session
* /Membres/crossLogin/{token} => return used id from token
* /Membres/peuxAppeller/{destination:[0-9]+}/{token:[a-zA-Z0-9]+} => return 1 or 0
* /Groupes/envoieAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+} => return 1 or 0
* /Groupes/ecouteAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+} => return 1 or 0

The server implement the message source at address

/messages?CSRFtoken=token;

*newCall => {from: userid}
*declineCall => {from: userid}
*acceptedCall => {from: userid}

# Without side site

if enable is false, the identification use asymetrique cryptography to identify users to each other 

/getCallTicket headers["PKey" : public key ] => ticket

/messages?PKey=public key

*newCall => {from: public key, challenge1 }
*answer => {from: public key, challenge1signed, challenge2 }
*answer2 => {from: public key, challenge2signed, challenge3 }
*declineCall => {from: public key, challenge3signed}
*acceptedCall => {from: public key, challenge3signed}
