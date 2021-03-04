
        var streamServer = 'localhost:8080';


        var globalAudio = { playing : false, downstreamURL:'http://'+streamServer + '/joinRoom',totalSamplesDecoded :0,
                            recording : false, upstreamURL:'ws://'+streamServer + '/upRoom',totalSent:0,
                            downCallURL:'http://'+streamServer + '/joinCall',FetchCallcontroller:null,
                            upCallURL:'ws://'+streamServer + '/upCall',
                            token:null,
                            Fetchcontroller:null,
                            audioContext:null}
                

        function convertoFloat32ToInt16(buffer) {
            var l = buffer.length;  //Buffer
            var buf = new Int16Array(l);

            while (l--) {
                s = Math.max(-1, Math.min(1, buffer[l]));
                buf[l] = s < 0 ? s * 0x8000 : s * 0x7FFF;
            }
          
            return buf.buffer;
        }

        function startCall(roomID, token)
        {
            globalAudio.webSocket = new WebSocket(globalAudio.upCallURL + "?token=" + token + "&roomID=" + roomID);
            globalAudio.webSocket.binaryType = 'arraybuffer';

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new AudioContext();

            globalAudio.totalSent = 0
            globalAudio.calling  = true;

            navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {

                globalAudio.stream = stream;

                globalAudio.audioInput = globalAudio.audioContext.createMediaStreamSource(stream);
			    globalAudio.gainNode = globalAudio.audioContext.createGain();
			    globalAudio.recorder = globalAudio.audioContext.createScriptProcessor(1024, 1, 1);

    			globalAudio.recorder.onaudioprocess = function(e) {

                    if(globalAudio.calling == false)return;

	    			var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                    globalAudio.webSocket.send(packets, { binary: true });
                    
                    globalAudio.totalSent += e.inputBuffer.getChannelData(0).length;
			    }
                globalAudio.audioInput.connect(globalAudio.recorder);
                //globalAudio.audioInput.connect(globalAudio.gainNode);
                //globalAudio.gainNode.connect(globalAudio.recorder);

                globalAudio.startTime=Date.now();

                globalAudio.recorder.connect(globalAudio.audioContext.destination);
            });
        }

        function playCall(roomID, token){

            var audioStack = [];
            var nextTime = 0;

            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            globalAudio.FetchCallcontroller = new AbortController();
            const { signal } = globalAudio.FetchCallcontroller;

            globalAudio.calling  = true;
    
            var opusURL = globalAudio.downCallURL + "?roomID=" + roomID; 
          
            // Fetch a file and decode it.
            fetch(opusURL, {signal, headers : { 'CSRFToken': token}})
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
                    if(globalAudio.calling == false)return;

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


        function playCallWav(roomID, token){

            var audioStack = [];
            var nextTime = 0;
            var leftByte = null

        
          
            globalAudio.calling = true                           
          
            if(globalAudio.audioContext == null)
                globalAudio.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            globalAudio.Fetchcontroller = new AbortController();
            const { signal } = globalAudio.Fetchcontroller;                

            var url= globalAudio.downstreamURL + "?format=wav&roomID=" + roomID; 

            fetch(url, {signal,headers : { 'CSRFToken': token}}).then(function(response) {

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

                        if(globalAudio.calling == false)return

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

        function stopCall()
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

            globalAudio.calling = false;
            globalAudio.webSocket.close();

            globalAudio.FetchCallcontroller.abort();
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

                    globalAudio.totalSent += e.inputBuffer.getChannelData(0).length;
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

            
            globalAudio.Fetchcontroller = new AbortController();
            const { signal } = globalAudio.Fetchcontroller;
    

            var opusURL = globalAudio.downstreamURL + "?roomID=" + roomID; // + "&token=" + token;
          
            // Fetch a file and decode it.
            fetch(opusURL, {signal,headers : { 'CSRFToken': token}})
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

            globalAudio.Fetchcontroller = new AbortController();
            const { signal } = globalAudio.Fetchcontroller;                

            var url= globalAudio.downstreamURL + "?format=wav&roomID=" + roomID; 

            fetch(url, {signal,headers : { 'CSRFToken': token}}).then(function(response) {

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
            globalAudio.Fetchcontroller.abort();
        }
