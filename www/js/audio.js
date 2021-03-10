
        var streamServer = 'localhost:8080';
        

        var globalAudio = { playing : false, downstreamURL:'http://'+streamServer + '/joinRoom',totalRecv:0,totalSamplesDecoded :0,
                            recording : false, upstreamURL:'ws://'+streamServer + '/upRoom',totalSent:0,totalSamplesSent :0,
                            downCallURL:'http://'+streamServer + '/joinCall',FetchCallcontroller:null,
                            upCallURL:'ws://'+streamServer + '/upCall',
                            token:null,pubkey:null,
                            Fetchcontroller:null,stream:null,
                            audioContext:null}
                


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

        function startCall(otherID)
        {
            if(globalAudio.recording)return

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new AudioContext();

            globalAudio.totalSent = 0
            globalAudio.totalSamplesSent = 0
            globalAudio.calling  = true;

            if( globalAudio.token != null){
                globalAudio.webSocket = new WebSocket(globalAudio.upCallURL + "?token=" + globalAudio.token + "&otherID=" + otherID);
            }else{
                globalAudio.webSocket = new WebSocket(globalAudio.upCallURL + "?PKey=" + globalAudio.pubkey + "&otherID=" + otherID);
            }
            globalAudio.webSocket.binaryType = 'arraybuffer';

            globalAudio.webSocket.onerror = function () { console.log('error starting call')}

            globalAudio.webSocket.onopen = function () {
               
                navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {

                    globalAudio.stream = stream;

                    globalAudio.audioInput = globalAudio.audioContext.createMediaStreamSource(stream);
                    globalAudio.gainNode = globalAudio.audioContext.createGain();
                    globalAudio.recorder = globalAudio.audioContext.createScriptProcessor(1024, 1, 1);

                    globalAudio.recorder.onaudioprocess = function(e) {

                        if(globalAudio.calling == false)return;

                        var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                        globalAudio.webSocket.send(packets, { binary: true });
                        
                        globalAudio.totalSamplesSent += e.inputBuffer.getChannelData(0).length;
                        globalAudio.totalSent += packets.byteLength;
                    }
                    globalAudio.audioInput.connect(globalAudio.recorder);

                    globalAudio.startTime=Date.now();
                    globalAudio.recorder.connect(globalAudio.audioContext.destination);

                    globalAudio.recording = true;
                });
            };
        }

        function playCall(otherID){

            var audioStack = [];
            var nextTime = 0;

            if(globalAudio.playing)
                return

            globalAudio.playing  = true;
            globalAudio.calling = true;

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            
            var opusURL = globalAudio.downCallURL + "?otherID=" + otherID; 

            if(globalAudio.token != null)
            {
                hdr = { 'CSRFToken': globalAudio.token};
            }
            else
            {
                hdr = { 'PKey': globalAudio.pubkey};
            }

            globalAudio.FetchController = new AbortController();

           

            try {
                // Fetch a file and decode it.
                fetch(opusURL, { signal:  globalAudio.FetchController.signal, headers : hdr})
                .then(decodeOpusResponse)
                .catch(console.error);
            }
            catch(err)
            {
                if (err.name == 'AbortError') { 
                    
                } else {
                    globalAudio.playing = false;
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
                globalAudio.totalRecv =0

                const decoder = new OpusStreamDecoder({onDecode});
                const reader = response.body.getReader();

                // TODO fail on decode() error and exit read() loop
                return reader.read().then(async function evalChunk({done, value}) 
                {
                    if ((done)||(globalAudio.calling == false)){
                        return;
                    }

                    globalAudio.totalRecv += value.byteLength

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

                    var myArrayBuffer = globalAudio.audioContext.createBuffer(1,frameCount, sampleRate);

                    var nowBuffering = myArrayBuffer.getChannelData(0);
                    for (var i = 0; i < frameCount; i++) {
                        nowBuffering[i] = buffer[i];
                    }                    

                    var source    = globalAudio.audioContext.createBufferSource();
                    source.buffer = myArrayBuffer;
                    source.connect(globalAudio.audioContext.destination);
                    if (nextTime == 0)
                        nextTime = globalAudio.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like

                    source.start(nextTime);

                    nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                }
                globalAudio.totalSamplesDecoded+=samplesDecoded;
            }
        }


        function playCallWav(otherID){

            var audioStack = [];
            var nextTime = 0;
            var leftByte = null

            if(globalAudio.playing)
                return
        
            globalAudio.playing = true;
            globalAudio.calling = true                           
          
            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();

         


            var url= globalAudio.downCallURL + "?format=wav&otherID=" + otherID; 

            if(globalAudio.token != null)
            {
                hdr = { 'CSRFToken': globalAudio.token};
            }
            else
            {
                hdr = { 'PKey': globalAudio.pubkey};
            }
            
            globalAudio.FetchController = new AbortController();

            try {

                globalAudio.playing = true;

                fetch(url, { signal:  globalAudio.FetchController.signal, headers : hdr})
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

                    globalAudio.totalRecv =0

                    var reader = response.body.getReader();

                    function myread(){

                        reader.read().then(({ value, done })=> {

                            if ((done)||(globalAudio.calling == false)){
                                return;
                            }

                            globalAudio.totalRecv += value.byteLength
        
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
                                
                                var myArrayBuffer = globalAudio.audioContext.createBuffer(1, frameCount , 48000);
                                var nowBuffering = myArrayBuffer.getChannelData(0);
            
                                for (var i = 0; i < frameCount; i++) {
                                    nowBuffering[i] = buffer[i] / 32768.0;
                                }               
                                
                                var source    = globalAudio.audioContext.createBufferSource();
                                source.buffer = myArrayBuffer;

                                source.connect(globalAudio.audioContext.destination);
                                if (nextTime == 0)
                                    nextTime = globalAudio.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like
                                    
                                source.start(nextTime);
                                nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                            }
                            myread();
                        });
                    }
                    myread();
                })
            }
            catch(err)
            {
                if (err.name == 'AbortError') { 
                    
                } else {
                    globalAudio.recording = false;
                  throw err;
                }
            }
        }
        function stopCallmic()
        {
            if( globalAudio.recording == false)
                return;
            
            globalAudio.recording = false;

            if(globalAudio.stream)
                globalAudio.stream.getTracks().forEach(function(track) { track.stop(); });

            if (globalAudio.audioInput) {
                globalAudio.audioInput.disconnect();
                globalAudio.audioInput = null;
            }
            if (globalAudio.gainNode) {
                globalAudio.gainNode.disconnect();
                globalAudio.gainNode = null;
            }
            if (globalAudio.recorder) {
                globalAudio.recorder.disconnect();
                globalAudio.recorder = null;
            }

            if(globalAudio.webSocket)
            {
                globalAudio.webSocket=null
                globalAudio.webSocket.close();
            }
        }

        function stopCallhds()
        {
            if( globalAudio.playing == false)
                return;

            globalAudio.playing = false;

            if(globalAudio.FetchController)
                globalAudio.FetchController.abort();

            globalAudio.FetchController = null;
        }

        function stopCall()
        {
            if(globalAudio.calling == false)
                return;

            globalAudio.calling = false;

            stopCallmic();
            stopCallhds();
       }

        function startReccording(roomID, token)
        {
            if(globalAudio.recording == true)
            {
                $('#reccord').html('reccord');
                stopReccording();
                return;
            }
            globalAudio.recording = true;
            $('#reccord').html('stop');

            globalAudio.webSocket = new WebSocket(globalAudio.upstreamURL + "?token=" + token + "&roomID=" + roomID);
            globalAudio.webSocket.binaryType = 'arraybuffer';

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new AudioContext();

            navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {

                globalAudio.stream = stream;

                globalAudio.audioInput = globalAudio.audioContext.createMediaStreamSource(stream);
			    globalAudio.gainNode = globalAudio.audioContext.createGain();
			    globalAudio.recorder = globalAudio.audioContext.createScriptProcessor(1024, 1, 1);

    			globalAudio.recorder.onaudioprocess = function(e) {

	    			var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                    globalAudio.webSocket.send(packets, { binary: true });
                    
                    /*
                    console.log('packet len '+packets.byteLength +' '+e.inputBuffer.getChannelData(0).length);
                    console.log('stat');
                    console.log((globalAudio.totalSent) / ((Date.now()-globalAudio.startTime)/1000));
                    */

                   globalAudio.totalSamplesSent += e.inputBuffer.getChannelData(0).length;
                   globalAudio.totalSent += packets.byteLength;
			    }
                globalAudio.audioInput.connect(globalAudio.recorder);
                //globalAudio.audioInput.connect(globalAudio.gainNode);
                //globalAudio.gainNode.connect(globalAudio.recorder);

                
                globalAudio.startTime=Date.now();

                globalAudio.recorder.connect(globalAudio.audioContext.destination);
            });
        }


        function stopReccording()
        {
            if (globalAudio.audioInput) {
				globalAudio.audioInput.disconnect();
				globalAudio.audioInput = null;
			}
			if (globalAudio.gainNode) {
				globalAudio.gainNode.disconnect();
				globalAudio.gainNode = null;
			}
			if (globalAudio.recorder) {
				globalAudio.recorder.disconnect();
				globalAudio.recorder = null;
			}

            globalAudio.recording = false;
            globalAudio.webSocket.close();
        }

        
    

        function playOpus(roomID, token){

            var audioStack = [];
            var nextTime = 0;

            if(globalAudio.playing == false){
                $('#play').html('stop')
                globalAudio.playing = true                           
            }
            else
            {
                stoplaying();
                $('#play').html('play')
                return
            }                      

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            
    

            var opusURL = globalAudio.downstreamURL + "?roomID=" + roomID; // + "&token=" + token;
          
            // Fetch a file and decode it.
            fetch(opusURL, {headers : { 'CSRFToken': token}})
            .then(decodeOpusResponse)
            .then(_ => console.log('decoded '+globalAudio.totalSamplesDecoded+' samples.'))
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
                    if(globalAudio.playing == false)return;

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

                    var myArrayBuffer = globalAudio.audioContext.createBuffer(1,frameCount, sampleRate);

                    var nowBuffering = myArrayBuffer.getChannelData(0);
                    for (var i = 0; i < frameCount; i++) {
                        nowBuffering[i] = buffer[i];
                    }                    

                    var source    = globalAudio.audioContext.createBufferSource();
                    source.buffer = myArrayBuffer;
                    source.connect(globalAudio.audioContext.destination);
                    if (nextTime == 0)
                        nextTime = globalAudio.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like

                    source.start(nextTime);

                    nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                }
                globalAudio.totalSamplesDecoded+=samplesDecoded;
            }
        }
   
        function play(roomID, token) {

            var audioStack = [];
            var nextTime = 0;
            var leftByte = null

        
            if(globalAudio.playing == false){
                $('#play').html('stop')
                globalAudio.playing = true                           
            }
            else
            {
                stoplaying();
                $('#play').html('play')
                return
            }            

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();


            var url= globalAudio.downstreamURL + "?format=wav&roomID=" + roomID; 

            fetch(url, {headers : { 'CSRFToken': token}}).then(function(response) {

                console.log(response)

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

                var reader = response.body.getReader();

                function myread(){

                    reader.read().then(({ value, done })=> {

                        if(globalAudio.playing == false)return

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
                            
                            var myArrayBuffer = globalAudio.audioContext.createBuffer(1, frameCount , 48000);
                            var nowBuffering = myArrayBuffer.getChannelData(0);
        
                            for (var i = 0; i < frameCount; i++) {
                                nowBuffering[i] = buffer[i] / 32768.0;
                            }               
                            
                            var source    = globalAudio.audioContext.createBufferSource();
                            source.buffer = myArrayBuffer;

                            source.connect(globalAudio.audioContext.destination);
                            if (nextTime == 0)
                                nextTime = globalAudio.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like
                                
                            source.start(nextTime);
                            nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                        }
                        myread();
                    });
                }
                myread();
            })
        }

        function stoplaying()
        {
            globalAudio.playing = false
        }
