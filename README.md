# gostream


Go stream is a live streaming server that can serve VOIP and group calls

It can function in two modes with a side website that can provide and check identification token linked to user session or in standalone mode using asymetric cryptography

# With side site


## The site definition is on the first line of hello.go

`var mysite site = site{siteURL: "http://localhost", siteOrigin: "http://localhost", enable: true}`

siteOrigin is the domain of the site that host the page that will use the streaming api with CORS requests

siteURL is the base URL for the site API 

* /Membres/newCRSF => return a token to be used in subsequent request based on user session
* /Membres/crossLogin/{token} => return used id from token
* /Membres/peuxAppeller/{destination:[0-9]+}/{token:[a-zA-Z0-9]+} => return 1 or 0
* /Groupes/envoieAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+} => return 1 or 0
* /Groupes/ecouteAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+} => return 1 or 0

## The server implement the message source at address

/messages?CSRFtoken=token;

* newCall => {from: userid}
* declineCall => {from: userid}
* acceptedCall => {from: userid}

## Server API

all server request using the site API must add the HTTP header "CSRFToken": token

### P2P calls

* /newCall [Destination : IDuser] 
* /rejectCall [Destination : IDuser] 
* /acceptCall [Destination : IDuser] 

* /upCall [Destination : IDuser]   <= raw audio data
* /joinCall [Destination : IDuser, format] => wav|ogg|opus audio data


### chat room
* /upRoom [RoomID : ID]  <= raw audio data
* /joinRoom [RoomID : ID, format] => wav|ogg|opus audio data

# Without side site

if enable is false, the identification use asymetrique cryptography to identify users to each other 

/getCallTicket headers["PKey" : public key ] => ticket

The server implement the message source at address

/messages?PKey=public key

* newCall => {from: public key, challenge1 }
* answer => {from: public key, challenge1signed, challenge2 }
* answer2 => {from: public key, challenge2signed, challenge3 }
* declineCall => {from: public key, challenge3signed}
* acceptedCall => {from: public key, challenge3signed}

## Server API

all server request using the cryptographic API must add the HTTP header "PKey": public key

### P2P calls

* /getCallTicket => serverChallenge

* /newCall   [Destination : public key, challenge1, serverChallengeSigned ] => serverChallenge

* /answer    [Destination : public key, challenge2, challenge1signed] 
* /answer2   [Destination : public key, challenge3, challenge2signed] 

* /rejectCall [Destination : public key, challenge3signed] 
* /acceptCall [Destination : public key, challenge3signed] 

* /upCall    [Destination : public key] <= raw audio data
* /joinCall  [Destination : public key] => wav|ogg|opus audio data





