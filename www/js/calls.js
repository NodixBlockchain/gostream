        function get_time_diff( earlierDate ){

            laterDate=new Date()
                        
            var oDiff = new Object();
                        
            //  Calculate Differences
            //  -------------------------------------------------------------------  //
            var nTotalDiff = laterDate.getTime() - earlierDate.getTime();
                        
            oDiff.days = Math.floor(nTotalDiff / 1000 / 60 / 60 / 24);
            nTotalDiff -= oDiff.days * 1000 * 60 * 60 * 24;
                        
            oDiff.hours = Math.floor(nTotalDiff / 1000 / 60 / 60);
            nTotalDiff -= oDiff.hours * 1000 * 60 * 60;
                        
            oDiff.minutes = Math.floor(nTotalDiff / 1000 / 60);
            nTotalDiff -= oDiff.minutes * 1000 * 60;
                        
            oDiff.seconds = Math.floor(nTotalDiff / 1000);
            //  -------------------------------------------------------------------  //
                        
            //  Format Duration
            //  -------------------------------------------------------------------  //
            //  Format Hours
            var hourtext = '00';
            if (oDiff.days > 0){ hourtext = String(oDiff.days);}
            if (hourtext.length == 1){hourtext = '0' + hourtext};
                        
            //  Format Minutes
            var mintext = '00';
            if (oDiff.minutes > 0){ mintext = String(oDiff.minutes);}
            if (mintext.length == 1) { mintext = '0' + mintext };
                        
            //  Format Seconds
            var sectext = '00';
            if (oDiff.seconds > 0) { sectext = String(oDiff.seconds); }
            if (sectext.length == 1) { sectext = '0' + sectext };
                        
            //  Set Duration
            var sDuration = hourtext + ':' + mintext + ':' + sectext;
            oDiff.duration = sDuration;
            //  -------------------------------------------------------------------  //
                    
            return oDiff;
        }
                                               
        function SzTxt(size)
        {
            if(size<1024)
                return size.toString(10) + " b"

            if(size<1024*1024)
              return (size.toString(10)/1024).toFixed(1) + " kb"

            return (size.toString(10)/(1024*1024)).toFixed(1) + " mb"              
        }

        function convertoFloat32ToInt16(buffer) {
            var l = buffer.length;  //Buffer
            var buf = new Int16Array(l);

            while (l--) {
                s = Math.max(-1, Math.min(1, buffer[l]));
                buf[l] = s < 0 ? s * 0x8000 : s * 0x7FFF;
            }
          
            return buf.buffer;
        }

        class StreamServer {
            
            constructor(streamServer, HTTPProto, WSProto){

                this.streamServer = streamServer
                this.HTTPProto = HTTPProto
                this.WSProto = WSProto                

                this.pubkey =null;
                this.token = null;
                this.serverChallenge = null;
            }

            init( pubkey )
            {
                var self=this;

                if(pubkey != null)
                {
                    this.pubkey=pubkey;
                    return $.ajax({url: this.HTTPProto+'://'+this.streamServer+'/getCallTicket', type : 'GET', headers : { 'PKey' : this.pubkey }, dataType: "text", success: function (challenge) { self.serverChallenge = challenge; } });
                    
                }else{
                    return $.getJSON('http://localhost/Membres/newCRSF', function(result)  { self.token = result.token; });            
                }
            }

            isValidId(DestinationID)
            {
                if(this.token == null)
                {
                    if(DestinationID.length<65)
                        return false; 
                }

                return true;
            }

            tokenCheck()
            {
                var self=this;

                return $.ajax({url:  this.HTTPProto + '://'+this.streamServer +'/tokenCheck', type: 'GET', headers: { 'CSRFToken': this.token}, dataType: "text",
                    success: function (result) {  self.userID = result; },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/tokenCheck"'); } 
                });
            }

            newCall(DestinationID, Challenge, Signature)
            {
                var self=this;
                var hdr={};
                
                if(!this.isValidId(DestinationID))
                    return null

                var postData ={Destination:DestinationID}

                if(this.token != null)
                {
                    hdr = { 'CSRFToken': this.token};
                }
                else
                {
                    hdr = { 'PKey': this.pubkey};

                    postData.challenge = Challenge;
                    postData.signature = Signature;
                }

                return $.ajax({url:  this.HTTPProto + '://'+this.streamServer +'/newCall', type: 'POST', headers: hdr, dataType: "text", data : postData, 
                    success: function (result) {  self.serverChallenge = result; },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/newCall"'); } 
                });
                
            }

            //from callee to caller
            answer(To, Challenge, Signature){
                var self=this;

                return $.ajax({url:this.HTTPProto + '://' +this.streamServer + '/answer', type : 'POST', headers : { 'PKey' : this.pubkey }, dataType : "text", data : { Destination : To , challenge : Challenge, signature : Signature},
                    success: function (result) { },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/answer"'); }
                }); 
            }

            //from caller to callee
            answer2(To, Challenge, Signature){
                var self=this;

                return $.ajax({url:this.HTTPProto + '://' +this.streamServer + '/answer2', type : 'POST', headers : { 'PKey' : this.pubkey }, dataType: "text", data: { Destination : To, challenge : Challenge, signature : Signature},
                    success: function (result) {  },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/answer2"'); }
                });     
            }

            acceptCall(From, Signature)
            { 
                var self=this;
                var hdr={};
                var postData={From:From}

                if(this.token != null)
                {
                    hdr = { 'CSRFToken':this.token};
                }
                else
                {
                    hdr = { 'PKey':this.pubkey};
                    postData.signature = Signature;
                }


                return $.ajax({url :this.HTTPProto + '://'+this.streamServer + '/acceptCall', type : 'POST', headers : hdr , dataType : "text", data : postData,
                    success: function (result) {  },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/acceptCall"'); }
                }); 
            }

            rejectCall(From,Signature)
            {
                var self=this;
                var hdr={};
                var postData = {From:From}
    
                if(this.token != null)
                {
                    hdr = { 'CSRFToken':this.token};
                }
                else
                {
                    hdr = { 'PKey':this.pubkey};
                    postData.signature = Signature;
                }
    
                return $.ajax({url:this.HTTPProto + '://'+this.streamServer +'/rejectCall', type : 'POST', headers : hdr, dataType: "text", data : postData,
                    success: function (result) {   },
                    error: function (error) { console.log('ajax error "'+error.responseText +'" on URL : "'+self.HTTPProto + '://'+self.streamServer +'/rejectCall"'); }
                }); 
            }

 
            
        }   

        class CallClient {

            hex_to_ascii(str1)
            {
                var hex  = str1.toString();
                var str = '';
                for (var n = 0; n < hex.length; n += 2) {
                    str += String.fromCharCode(parseInt(hex.substr(n, 2), 16));
                }
                return str;
            }

            createChallenge()
            {
                return  Math.floor(Math.random()*100000000);
            }
            
            constructor(mode = 'crypto'){

               this.callUpdateTimeout = null;
               this.server = null;

               this.playing = false;
               this.totalRecv=0;
               this.totalSamplesDecoded=0;

               this.recording = false;
               this.totalSent = 0;
               this.totalSamplesSent = 0;

               this.stream = null;
             
               this.callStart=null;
               this.audioContext = null;

               this.audioInput = null;
            
                this.gainNode = null;
                this.recorder = null;
                this.webSocket = null;

                if(mode == 'crypto'){
                    this.ec = new elliptic.ec('p256');
                    this.key = this.ec.genKeyPair();
                    this.pubkey = this.key.getPublic().encodeCompressed('hex');
                    this.Challenge = null;
                    this.enc = new TextEncoder(); // always utf-8
                }else{
                    this.ec = null;
                    this.key = null;
                    this.pubkey = null;
                }
            }


            initialize(streamServer)
            {
                var self=this;
                this.server = streamServer;

                var xhr = this.server.init(this.pubkey);

                if(xhr == null)
                    return;

                xhr.done(function()
                {
                    if(self.pubkey == null){

                        self.server.tokenCheck().done(function(){

                            self.eventSource = new EventSource(self.server.HTTPProto+'://'+self.server.streamServer+'/messages?CSRFtoken=' + self.server.token); 

                            self.eventSource.addEventListener('newCall',function(e){self.called(JSON.parse(e.data))});
                            self.eventSource.addEventListener('declineCall',function(e){self.callDeclined(JSON.parse(e.data))});
                            self.eventSource.addEventListener('acceptedCall',function(e){self.callAccepted(JSON.parse(e.data))});
                            self.eventSource.addEventListener('setAudioConf',function(e){self.setAudioConf(JSON.parse(e.data))});

                            self.meID = self.server.userID;
                            $('#moi').html( 'moi : ' + self.meID ); 
                        });
           
                    }else{
                        self.eventSource = new EventSource(self.server.HTTPProto+'://'+self.server.streamServer+'/messages?PKey=' + self.pubkey);

                        self.eventSource.addEventListener('newCall',function(e){self.called(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});
                        self.eventSource.addEventListener('declineCall',function(e){self.callDeclined(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});
                        self.eventSource.addEventListener('acceptedCall',function(e){self.callAccepted(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});
                        self.eventSource.addEventListener('setAudioConf',function(e){self.setAudioConf(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});

                        self.eventSource.addEventListener('answer',function(e){self.answered(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});
                        self.eventSource.addEventListener('answer2',function(e){self.answer2(JSON.parse(self.hex_to_ascii(JSON.parse(e.data))))});

                        $('#moi').html( 'moi : <span class="key">' + self.pubkey +'</span>');
                    }
                })
                .fail(function (error) { console.log('cannot initialize stream server @' + self.server.streamServer ); });
            }


            
            updateCallInfos() 
            { 
                var timeDiff = get_time_diff(this.callStart)
                                
                $('#call-time').html(timeDiff.duration) 
                $('#call-up').html(SzTxt(this.totalSent)) 
                $('#call-down').html(SzTxt(this.totalRecv)) 
            }

            /* UI buttons events */
            call(DestinationID)
            {
                var xhr;

                if(this.server.token != null) {

                    if(DestinationID == this.meID)    
                    {
                        $('#destination-error').html('cannot call self');
                        return false; 
                    }
                        
                    xhr = this.server.newCall(DestinationID);

                }else{

                    var okey = this.ec.keyFromPublic(DestinationID,'hex');
                    if(!okey)
                    {
                        $('#destination-error').html('invalid destination');
                        return false; 
                    }
    
                    if(okey.getPublic().encodeCompressed('hex') == this.pubkey)
                    {
                        $('#destination-error').html('cannot call self');
                        return false;   
                    }
                        
                    this.Challenge = this.createChallenge();
                    var signature = this.key.sign(this.enc.encode(this.server.serverChallenge)).toDER('hex');
                    xhr = this.server.newCall(DestinationID,this.Challenge,signature);
                }

                if(!xhr)
                {
                    $('#destination-error').html('unable to connect stream server');
                    return;    
                }            

                $('#destination-error').html('');
                
                xhr.done(function(){
                    $('#appel-calling').css('display','inline');
                    $('#appel-calling').html('calling ...');

                    $('#appel-decline').css('display','none');   
                    $('#appelModal1Label').html('Appel <span class="key">'+ DestinationID)+'</span>'; 
                    $('#appelModal1').modal(); 
                });
            }

            
            acceptCall(From,challenge)
            {
                var self=this;
                var xhr;

                if(this.server.token != null) {
                    xhr = this.server.acceptCall(From);
                }else{
                    var Signature = this.key.sign(this.enc.encode(challenge)).toDER('hex');
                    xhr = this.server.acceptCall(From, Signature);
                }

                if(!xhr)
                    return;

                xhr.done(function (result)  {  

                    if(self.server.token != null){
                        self.playCall(From);  
                    }else{
                        self.playCallWav(From);  
                    }

                    $('#call-hds').removeClass('badge-danger');  
                    $('#call-hds').addClass('badge-success');
                        
                    $('#appelModal').modal('hide')
                    $('#appelEnCoursModal').modal();


                    $('#call-time').html('00:00:00');
                    $('#call-up').html(0) ;
                    $('#call-down').html(0);

                    if(self.callUpdateTimeout == null){
                        self.callUpdateTimeout = setInterval(function(){ self.updateCallInfos(); } , 1000)                    
                    }
                }); 
            }   
            
            rejectCall(From, challenge)
            {
                var xhr;
               
                if(this.server.token != null) {
                    xhr = this.server.rejectCall(From);
                }else{
                    var Signature = this.key.sign(this.enc.encode(challenge)).toDER('hex');
                    xhr = this.server.rejectCall(From, Signature);
                }

                if(!xhr)
                    return;

                xhr.done( function (result) {   
                    $('#call-infos').html('call from <span class="key">'+From+'</span> rejected ') 
                    $('#accept-call-btn').prop('disabled','disabled'); 
                    $('#reject-call-btn').prop('disabled','disabled'); 
                    
                }); 
            }
            
            /* message clients event */

            /* caller event */
            answered(data) {

                var okey = this.ec.keyFromPublic(data.from,'hex');

                if(!okey.verify(this.enc.encode(this.Challenge),data.answer)){
                    console.log('verify caller origin failed '+this.Challenge+' \n')
                    console.log(data.from)
                    console.log(data.answer)
                    $('#appel-calling').html('failed to verify key from <span class="key">'+data.from+'</span>');
                    return;
                }

                this.Challenge = this.createChallenge();
                var Signature = this.key.sign(this.enc.encode(data.challenge)).toDER('hex')
                return this.server.answer2(data.from, this.Challenge, Signature);
            }

            
             /* caller event */
             callAccepted(data)
             {
                 var self=this;
                 if(this.server.token == null)
                 {
                     var okey = this.ec.keyFromPublic(data.from,'hex');
                 
                     if(!okey.verify(this.enc.encode(this.Challenge),data.answer)){
                         console.log('verify check failed '+this.Challenge)
                         console.log(data.from)
                         console.log(data.answer)
                         $('#appel-calling').html('key verification failed <span class="key">'+data.from+'</span>');   
                         return; 
                     }
                 }
                 

                 $('#callerID').val(data.from); 

                 this.startCall(data.from); 
 
                 $('#call-mic').removeClass('badge-danger');  
                 $('#call-mic').addClass('badge-success');
 
                 $('#appelModal1').modal('hide')
                 $('#appelEnCoursModal').modal(); 
 
                 $('#call-time').html('00:00:00');
                 $('#call-up').html(0) ;
                 $('#call-down').html(0);
 
                 if(this.callUpdateTimeout == null){
                     this.callUpdateTimeout = setInterval( function(){ self.updateCallInfos(); }, 1000)      
                 }
             
             }        
             
             /* caller event */
             callDeclined(data)
             {
                 if(this.server.token == null)
                 {
                     var okey = this.ec.keyFromPublic(data.from,'hex');
 
                     if(!okey.verify(this.enc.encode(this.Challenge),data.answer)){
                         console.log('verify check failed '+this.Challenge)
                         console.log(data.from)
                         console.log(data.answer)
                         $('#appel-calling').html('key verification failed <span class="key">'+data.from+'</span>'); 
                         return;
                     }
                 }
                     
                 $('#appel-calling').css('display','none'); 
                 $('#appel-decline').css('display','inline'); 
                 $('#appel-decline').html('<span class="key">'+data.from+ '</span> a decliné l\'appel ');
             }
 


            /* callee event */
            answer2(data) 
            {
                var okey = this.ec.keyFromPublic(data.from,'hex');

                if(!okey.verify(this.enc.encode(this.Challenge),data.answer)){
                    console.log('verify called origin '+this.Challenge+' \n');
                    console.log(data.from);
                    console.log(data.answer);
                    return;
                }

                $('#callerID').val(data.from); 
                $('#callerChallenge').val(data.challenge)

                $('#call-infos').empty();

                $('#accept-call-btn').prop('disabled',false); 
                $('#reject-call-btn').prop('disabled',false); 


                $('#appelModalLabel').html('Appel de <span class="key">'+ data.from+'</span>'); 
                $('#appelModal').modal(); 
            }


             /* callee event */
            called(data) 
            {           
                if(this.server.calling)
                {
                    rejectCall(data.from,data.challenge)
                    return
                }

                if(this.server.token != null)
                {
                    $('#callerID').val(data.from); 

                    $('#call-infos').empty();

                    $('#accept-call-btn').prop('disabled',false); 
                    $('#reject-call-btn').prop('disabled',false); 

                    $('#appelModalLabel').html('Appel de '+ data.from); 
                    $('#appelModal').modal(); 
                    return;                
                }


                this.Challenge = this.createChallenge();
                var Signature = this.key.sign(this.enc.encode(data.challenge)).toDER('hex')
                return this.server.answer(data.from, this.Challenge, Signature);
            }
            

            setAudioConf(data)
            {
                if(data.in == 1)
                {
                    $('#call-audio-in').removeClass('badge-danger');  
                    $('#call-audio-in').addClass('badge-success');

                }
                else
                {
                    $('#call-audio-in').removeClass('badge-success');  
                    $('#call-audio-in').addClass('badge-danger');
                }
                
                if(data.out == 1)
                {
                    $('#call-audio-out').removeClass('badge-danger');  
                    $('#call-audio-out').addClass('badge-success');

                }
                else
                {
                    $('#call-audio-out').removeClass('badge-success');  
                    $('#call-audio-out').addClass('badge-danger');
                }            
            }


            startCall(otherID)
            {
                var self=this;

                if(this.recording)return

                if(this.callStart == null)
                    this.callStart = new Date();
    
                if(this.audioContext == null)
                    this.audioContext = new AudioContext();
    
                this.totalSent = 0
                this.totalSamplesSent = 0
                this.calling  = true;
    
                if( this.server.token != null){
                    this.webSocket = new WebSocket(this.server.WSProto +'://'+this.server.streamServer + '/upCall?token=' + this.server.token + "&otherID=" + otherID);
                }else{
                    this.webSocket = new WebSocket(this.server.WSProto +'://'+this.server.streamServer + '/upCall?PKey=' + this.pubkey + "&otherID=" + otherID);
                }
                this.webSocket.binaryType = 'arraybuffer';
    
                this.webSocket.onerror = function () { console.log('error starting call')}
    
                this.webSocket.onopen = function () {
                   
                    navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {
    
                        self.stream = stream;
    
                        self.audioInput = self.audioContext.createMediaStreamSource(stream);
                        self.gainNode = self.audioContext.createGain();
                        self.recorder = self.audioContext.createScriptProcessor(1024, 1, 1);
    
                        self.recorder.onaudioprocess = function(e) {
    
                            if(self.calling == false)return;
    
                            var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                            self.webSocket.send(packets, { binary: true });
                            
                            self.totalSamplesSent += e.inputBuffer.getChannelData(0).length;
                            self.totalSent += packets.byteLength;
                        }
                        self.audioInput.connect(self.recorder);
    
                        if(self.startTime == null)
                            self.startTime = new Date();


                        self.recorder.connect(self.audioContext.destination);
    
                        self.recording = true;
                    });
                };
            }
            
        

            playCall(otherID){

                var self=this;
                var hdr={};
                var audioStack = [];
                var nextTime = 0;

                if(this.playing)
                    return

                this.playing  = true;
                this.calling = true;

                if(this.audioContext == null)
                    this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

                
                if(this.server.token != null)
                {
                    hdr = { 'CSRFToken': this.server.token};
                }
                else
                {
                    hdr = { 'PKey': this.pubkey};
                }

                this.FetchController = new AbortController(); 

                if(this.callStart == null)
                    this.callStart = new Date();

                try {
                    // Fetch a file and decode it.
                    fetch(this.server.HTTPProto + '://'+this.server.streamServer + '/joinCall?otherID=' + otherID, { signal:  this.FetchController.signal, headers : hdr})
                    .then(decodeOpusResponse)
                    .catch(console.error);
                }
                catch(err)
                {
                    if (err.name == 'AbortError') { 
                        
                    } else {
                        this.playing = false;
                    throw err;
                    }
                }

                // decode Fetch response
                function decodeOpusResponse(response) {

                    var contentType =''

                    if (!response.ok)
                    throw Error('Invalid Response: '+response.status+' '+response.statusText)
                    if (!response.body)
                    throw Error('ReadableStream not yet supported in this browser.');

                    for(let entry of response.headers.entries()) {
                        if(entry[0] == 'content-type')
                            contentType = entry[1]
                    }      

                    if(contentType != 'audio/ogg')
                    {
                        alert('wrong content type '+contentType)
                        alert(response.body)
                        return;
                    }
                    self.totalRecv =0

                    const decoder = new OpusStreamDecoder({onDecode});
                    const reader = response.body.getReader();

                    // TODO fail on decode() error and exit read() loop
                    return reader.read().then(async function evalChunk({done, value}) 
                    {
                        if ((done)||(self.calling == false)){
                            return;
                        }

                        self.totalRecv += value.byteLength

                        await decoder.ready;
                        decoder.decode(value);

                        return reader.read().then(evalChunk);
                    })
                }

                // Callback that receives decoded PCM OpusStreamDecodedAudio
                function onDecode({left, right, samplesDecoded, sampleRate}) {

                    audioStack.push(left);

                    while ( audioStack.length) {
                        var buffer    = audioStack.shift();
                        var frameCount =  buffer.length;

                        var myArrayBuffer = self.audioContext.createBuffer(1,frameCount, sampleRate);

                        var nowBuffering = myArrayBuffer.getChannelData(0);
                        for (var i = 0; i < frameCount; i++) {
                            nowBuffering[i] = buffer[i];
                        }                    

                        var source    = self.audioContext.createBufferSource();
                        source.buffer = myArrayBuffer;
                        source.connect(self.audioContext.destination);
                        if (nextTime == 0)
                            nextTime = self.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like

                        source.start(nextTime);

                        nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                    }
                    self.totalSamplesDecoded+=samplesDecoded;
                }
            }


            playCallWav(otherID){
                var self=this;
                var hdr={};
                var audioStack = [];
                var nextTime = 0;
                var leftByte = null

                if(this.playing)
                    return
            
                this.playing = true;
                this.calling = true                           
            
                if(this.audioContext == null)
                    this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

                if(this.server.token != null)
                {
                    hdr = { 'CSRFToken': this.server.token};
                }
                else
                {
                    hdr = { 'PKey': this.pubkey};
                }
                
                this.FetchController = new AbortController();
                this.playing = true;

                if(this.callStart == null)
                    this.callStart = new Date();                

                

                fetch(this.server.HTTPProto + '://'+this.server.streamServer + '/joinCall' + "?format=wav&otherID=" + otherID, { signal:  this.FetchController.signal, headers : hdr})
                .then(function(response) {

                        if(!response.ok){
                            console.log('play call wav error ') 
                            console.log(response)
                            return
                        }
                        var contentType =''

                        for(let entry of response.headers.entries()) {
                            if(entry[0] == 'content-type')
                                contentType = entry[1]
                        }                

                        if(contentType != 'audio/wav')
                        {
                            alert('wrong content type '+contentType)
                            alert(response.body.readAll())
                            return;
                        }

                        self.totalRecv =0

                        var reader = response.body.getReader();

                        function myread(){

                            try {

                                reader.read().then(({ value, done })=> {

                                    if ((done)||(self.calling == false)){
                                        return;
                                    }

                                    self.totalRecv += value.byteLength
                
                                    audioStack.push(value.buffer);

                                    while ( audioStack.length) {

                                        var obuf       = audioStack.shift();
                                        var buffer;
                                    
                                        if((obuf.byteLength & 1) == 0)
                                        {
                                            if(leftByte != null)
                                            {
                                                var byteArray = new Uint8Array(obuf.byteLength);

                                                byteArray[0] = leftByte[0]
                                                byteArray.set(obuf.slice(0,-1), 1)
                                                buffer = new Int16Array(byteArray);
                                                leftByte= obuf.slice(-1)
                                            }
                                            else
                                            {
                                                buffer    = new Int16Array(obuf);
                                            }
                                        }
                                        else
                                        {
                                            if(leftByte != null)
                                            {                  
                                                var byteArray = new Uint8Array(obuf.byteLength+1);
                                                
                                                byteArray[0] = leftByte[0]
                                                byteArray.set(obuf, 1)
                                                buffer = new Int16Array(byteArray);
                                                leftByte = null;
                                            }
                                            else
                                            {                                
                                                buffer = new Int16Array(obuf.slice(0,-1));
                                                leftByte = obuf.slice(-1);
                                            }                                
                                        }
                                            

                                        var frameCount = buffer.length;
                                        
                                        self.totalSamplesDecoded+=frameCount;
                                        
                                        var myArrayBuffer = self.audioContext.createBuffer(1, frameCount , 48000);
                                        var nowBuffering = myArrayBuffer.getChannelData(0);
                    
                                        for (var i = 0; i < frameCount; i++) {
                                            nowBuffering[i] = buffer[i] / 32768.0;
                                        }               
                                        
                                        var source    = self.audioContext.createBufferSource();
                                        source.buffer = myArrayBuffer;

                                        source.connect(self.audioContext.destination);
                                        if (nextTime == 0)
                                            nextTime = self.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like
                                            
                                        source.start(nextTime);
                                        nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                                    }
                                    myread();
                                });
                            }
                            catch(err)
                            {
                                if (err.name == 'AbortError') { 
                                    
                                } else {
                                    self.recording = false;
                                throw err;
                                }
                            }
                        }
                        myread();
                    })
            }

            stopCallmic()
            {
                if( this.recording == false)
                    return;
                
                this.recording = false;

                if(this.stream)
                {
                    this.stream.getTracks().forEach(function(track) { track.stop(); });
                    this.stream=null;
                }
                    

                if (this.audioInput) {
                    this.audioInput.disconnect();
                    this.audioInput = null;
                }
                if (this.gainNode) {
                    this.gainNode.disconnect();
                    this.gainNode = null;
                }
                if (this.recorder) {
                    this.recorder.disconnect();
                    this.recorder = null;
                }

                if(this.webSocket)
                {
                    this.webSocket.close();
                    this.webSocket=null
                }
            }

            stopCallhds()
            {
                if( this.playing == false)
                    return;

                this.playing = false;

                if(this.FetchController)
                {
                    this.FetchController.abort();
                    this.FetchController = null;
                }

            }

            stopCall()
            {
                if(this.calling == false)
                    return;

                this.calling = false;

                this.stopCallmic();
                this.stopCallhds();

                this.callStart = null;
                

                if(this.callUpdateTimeout)
                {
                    clearInterval(this.callUpdateTimeout);
                    this.callUpdateTimeout=null;
                }
                    
            }            
        }

        class GroupClient {
            
            constructor(mode = 'crypto'){

                this.callUpdateTimeout == null;

                this.server = null;
                this.audioContext = null;
                this.stream = null;
                this.startTime = null;

                this.recording = false;
                this.webSocket = null;
                this.audioInput = null;
                this.gainNode = null;
                this.recorder =null;

                this.totalRecv = 0;
                this.totalSamplesDecoded = 0;

                this.totalSent = 0;
                this.totalSamplesSent = 0;

                if(mode == 'crypto'){
                    this.ec = new elliptic.ec('p256');
                    this.key = this.ec.genKeyPair();
                    this.pubkey = this.key.getPublic().encodeCompressed('hex');
                    this.Challenge = null;
                    this.enc = new TextEncoder(); // always utf-8
                }else{
                    this.ec = null;
                    this.key = null;
                    this.pubkey = null;
                }


                this.playing = false;
            }

            updateGroupInfos() 
            { 
                var timeDiff = get_time_diff(this.startTime)
                                
                $('#group-time').html(timeDiff.duration) 
                $('#group-up').html(SzTxt(this.totalSent)) 
                $('#group-down').html(SzTxt(this.totalRecv)) 
            }
            
            initialize(streamServer)
            {
                this.server = streamServer;
                return this.server.init(this.pubkey);
            }

            
            startReccording(roomID)
            {
                var self=this;
                if(this.recording == true)
                {
                    this.stopReccording();
                    return;
                }

                this.recording = true;
                
                $('#reccord').removeClass('badge-danger');  
                $('#reccord').addClass('badge-success');
                

                if( this.server.token != null){
                    this.webSocket = new WebSocket(this.server.WSProto +'://'+this.server.streamServer + '/upRoom?token=' + this.server.token + '&roomID=' + roomID);
                }else{
                    this.webSocket = new WebSocket(this.server.WSProto +'://'+this.server.streamServer + '/upRoom?PKey=' + this.pubkey + '&roomID=' + roomID);
                }


                
                this.webSocket.binaryType = 'arraybuffer';

                if(this.audioContext == null)
                    this.audioContext = new AudioContext();

                this.totalSent=0;
                this.totalSamplesSent=0;

                if(this.callUpdateTimeout == null){
                    this.callUpdateTimeout = setInterval( function(){ self.updateGroupInfos(); }, 1000)      
                }                

                this.webSocket.onerror = function () { console.log('error starting call')}
    
                this.webSocket.onopen = function () {
                    navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {

                        self.stream = stream;

                        self.audioInput = self.audioContext.createMediaStreamSource(stream);
                        self.gainNode = self.audioContext.createGain();
                        self.recorder = self.audioContext.createScriptProcessor(1024, 1, 1);

                        self.recorder.onaudioprocess = function(e) {

                            if(self.recording == false)return;

                            var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                            self.webSocket.send(packets, { binary: true });

                            self.totalSamplesSent += e.inputBuffer.getChannelData(0).length;
                            self.totalSent += packets.byteLength;
                        }
                        self.audioInput.connect(self.recorder);
                        //self.audioInput.connect(self.gainNode);
                        //self.gainNode.connect(self.recorder);
                    
                        if(self.startTime == null)
                            self.startTime = new Date();

                        self.recorder.connect(self.audioContext.destination);
                    });
                };
            }

            playOpus(roomID){

                var hdr={}
                var self=this;
                var audioStack = [];
                var nextTime = 0;

                if(this.playing == true){
                    this.stoplaying();
                    return;
                }
                this.playing = true;

                $('#play').removeClass('badge-danger');  
                $('#play').addClass('badge-success');

                if(this.audioContext == null)
                    this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

                this.FetchController = new AbortController(); 

                if(this.startTime == null)
                    this.startTime = new Date();

                this.totalRecv = 0;
                this.totalSamplesDecoded = 0;                

                if(this.callUpdateTimeout == null){
                   this.callUpdateTimeout = setInterval( function(){ self.updateGroupInfos(); }, 1000)      
                }                   

                if(this.server.token != null)
                {
                    hdr = { 'CSRFToken': this.server.token};
                }
                else
                {
                    hdr = { 'PKey': this.pubkey};
                }

                // Fetch a file and decode it.
                fetch(this.server.HTTPProto + '://'+this.server.streamServer + '/joinRoom?roomID=' + roomID, {signal:  this.FetchController.signal, headers : hdr})
                .then(decodeOpusResponse)
                .catch(console.error);

                // decode Fetch response
                function decodeOpusResponse(response) {

                    var contentType =''

                    if (!response.ok)
                    throw Error('Invalid Response: '+response.status+' '+response.statusText)
                    if (!response.body)
                    throw Error('ReadableStream not yet supported in this browser.');

                    for(let entry of response.headers.entries()) {
                        if(entry[0] == 'content-type')
                            contentType = entry[1]
                    }      

                    if(contentType != 'audio/ogg')
                    {
                        alert('wrong content type '+contentType)
                        alert(response.body)
                        return;
                    }

                    const decoder = new OpusStreamDecoder({onDecode});
                    const reader = response.body.getReader();

                    // TODO fail on decode() error and exit read() loop
                    return reader.read().then(async function evalChunk({done, value}) 
                    {
                        if (done) return;
                        if(self.playing == false)return;

                        self.totalRecv += value.byteLength

                        await decoder.ready;
                        decoder.decode(value);

                        return reader.read().then(evalChunk);
                    })
                }

                // Callback that receives decoded PCM OpusStreamDecodedAudio
                function onDecode({left, right, samplesDecoded, sampleRate}) {

                    audioStack.push(left);

                    while ( audioStack.length) {
                        var buffer    = audioStack.shift();
                        var frameCount =  buffer.length;

                        var myArrayBuffer = self.audioContext.createBuffer(1,frameCount, sampleRate);

                        var nowBuffering = myArrayBuffer.getChannelData(0);
                        for (var i = 0; i < frameCount; i++) {
                            nowBuffering[i] = buffer[i];
                        }                    

                        var source    = self.audioContext.createBufferSource();
                        source.buffer = myArrayBuffer;
                        source.connect(self.audioContext.destination);
                        if (nextTime == 0)
                            nextTime = self.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like

                        source.start(nextTime);

                        nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                    }
                    self.totalSamplesDecoded+=samplesDecoded;
                }
            }
   
            play(roomID) {
                var hdr={}
                var self=this;
                var audioStack = [];
                var nextTime = 0;
                var leftByte = null

            
                if(this.playing == true){
                    this.stoplaying();
                    return;
                }
                this.playing = true;

                $('#play').removeClass('badge-danger');  
                $('#play').addClass('badge-success');

    
                if(this.audioContext == null)
                    this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

                this.FetchController = new AbortController();

                if(this.startTime == null)
                    this.startTime = new Date();
                
                this.totalRecv = 0;
                this.totalSamplesDecoded = 0;

                if(this.callUpdateTimeout == null){
                    this.callUpdateTimeout = setInterval( function(){ self.updateGroupInfos(); }, 1000)      
                }                           

                if(this.server.token != null)
                {
                    hdr = { 'CSRFToken': this.server.token};
                }
                else
                {
                    hdr = { 'PKey': this.pubkey};
                }

                fetch(this.server.HTTPProto + '://'+this.server.streamServer + '/joinRoom?format=wav&roomID=' + roomID, { signal:  this.FetchController.signal,  headers : hdr } ).then(function(response) {

                    var contentType =''

                    for(let entry of response.headers.entries()) {
                        if(entry[0] == 'content-type')
                            contentType = entry[1]
                    }                

                    if(contentType != 'audio/wav')
                    {
                        alert('wrong content type '+contentType)
                        return;
                    }

                    var reader = response.body.getReader();

                    function myread(){

                        reader.read().then(({ value, done })=> {

                            if(self.playing == false)return

                            if ((done)||(self.calling == false)){
                                return;
                            }

                            self.totalRecv += value.byteLength

                            audioStack.push(value.buffer);

                            while ( audioStack.length) {

                                var obuf       = audioStack.shift();
                                var buffer;
                            
                                if((obuf.byteLength & 1) == 0)
                                {
                                    if(leftByte != null)
                                    {
                                        var byteArray = new Uint8Array(obuf.byteLength);

                                        byteArray[0] = leftByte[0]
                                        byteArray.set(obuf.slice(0,-1), 1)
                                        buffer = new Int16Array(byteArray);
                                        leftByte= obuf.slice(-1)
                                    }
                                    else
                                    {
                                        buffer    = new Int16Array(obuf);
                                    }
                                }
                                else
                                {
                                    if(leftByte != null)
                                    {                  
                                        var byteArray = new Uint8Array(obuf.byteLength+1);
                                        
                                        byteArray[0] = leftByte[0]
                                        byteArray.set(obuf, 1)
                                        buffer = new Int16Array(byteArray);
                                        leftByte = null;
                                    }
                                    else
                                    {                                
                                        buffer = new Int16Array(obuf.slice(0,-1));
                                        leftByte = obuf.slice(-1);
                                    }                                
                                }
                                    

                                var frameCount = buffer.length;    
                                
                                self.totalSamplesDecoded+=frameCount;
                                
                                var myArrayBuffer = self.audioContext.createBuffer(1, frameCount , 48000);
                                var nowBuffering = myArrayBuffer.getChannelData(0);
            
                                for (var i = 0; i < frameCount; i++) {
                                    nowBuffering[i] = buffer[i] / 32768.0;
                                }               
                                
                                var source    = self.audioContext.createBufferSource();
                                source.buffer = myArrayBuffer;

                                source.connect(self.audioContext.destination);
                                if (nextTime == 0)
                                    nextTime = self.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like
                                    
                                source.start(nextTime);
                                nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                            }
                            myread();
                        });
                    }
                    myread();
                })
            }

            stoplaying()
            {   
                if(this.playing == false)
                    return;

                this.playing = false;

                if((!this.recording)&&(this.callUpdateTimeout)){

                    clearInterval(this.callUpdateTimeout);
                    this.callUpdateTimeout = null;
                    this.startTime = null;
                }
                
                $('#play').removeClass('badge-success');  
                $('#play').addClass('badge-danger');

                if(this.FetchController)
                {
                    this.FetchController.abort();
                    this.FetchController = null;
                }
            }

            stopReccording()
            {
                if(this.recording == false)
                    return;

                this.recording = false;

                $('#reccord').removeClass('badge-success');  
                $('#reccord').addClass('badge-danger');

                if((!this.playing)&&(this.callUpdateTimeout)){

                    clearInterval(this.callUpdateTimeout);
                    this.callUpdateTimeout = null;
                    this.startTime = null;
                }

                if(this.stream)
                {
                    this.stream.getTracks().forEach(function(track) { track.stop(); });
                    this.stream=null;
                }

                if (this.audioInput) {
                    this.audioInput.disconnect();
                    this.audioInput = null;
                }
                if (this.gainNode) {
                    this.gainNode.disconnect();
                    this.gainNode = null;
                }
                if (this.recorder) {
                    this.recorder.disconnect();
                    this.recorder = null;
                }
             
                if(this.webSocket)
                {
                    this.webSocket.close();
                    this.webSocket=null;
                }
            }

        }

