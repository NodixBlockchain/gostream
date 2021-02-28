
        var g={playing : false, downstreamURL:'http://localhost:8081',totalSamplesDecoded :0,recording : false,totalSent:0, upstreamURL:'ws://localhost:8080',audioContext:null}
                

        function convertoFloat32ToInt16(buffer) {
            var l = buffer.length;  //Buffer
            var buf = new Int16Array(l);

            while (l--) {
                s = Math.max(-1, Math.min(1, buffer[l]));
                buf[l] = s < 0 ? s * 0x8000 : s * 0x7FFF;
            }
          
            return buf.buffer;
        }

        function startReccording(roomID)
        {
            if(g.recording == true)
            {
                $('#reccord').html('reccord');
                stopReccording();
                return;
            }
            g.recording = true;
            $('#reccord').html('stop');

            g.webSocket = new WebSocket(g.upstreamURL + "/joinRoom?roomID="+roomID);
            g.webSocket.binaryType = 'arraybuffer';

            if(g.audioContext == null)
                g.audioContext = new AudioContext();

            navigator.mediaDevices.getUserMedia ({audio: true, video: false}).then(function(stream) {

                g.stream = stream;

                g.audioInput = g.audioContext.createMediaStreamSource(stream);
			    g.gainNode = g.audioContext.createGain();
			    g.recorder = g.audioContext.createScriptProcessor(1024, 1, 1);

    			g.recorder.onaudioprocess = function(e) {

	    			var packets = convertoFloat32ToInt16(e.inputBuffer.getChannelData(0));
                    g.webSocket.send(packets, { binary: true });
                    
                    /*
                    console.log('packet len '+packets.byteLength +' '+e.inputBuffer.getChannelData(0).length);
                    console.log('stat');
                    console.log((g.totalSent) / ((Date.now()-g.startTime)/1000));
                    */

                    g.totalSent += e.inputBuffer.getChannelData(0).length;
			    }
                g.audioInput.connect(g.recorder);
                //g.audioInput.connect(g.gainNode);
                //g.gainNode.connect(g.recorder);

                
                g.startTime=Date.now();

                g.recorder.connect(g.audioContext.destination);
            });
        }


        function stopReccording()
        {
            if (g.audioInput) {
				g.audioInput.disconnect();
				g.audioInput = null;
			}
			if (g.gainNode) {
				g.gainNode.disconnect();
				g.gainNode = null;
			}
			if (g.recorder) {
				g.recorder.disconnect();
				g.recorder = null;
			}

            g.stream.getTracks()[0].stop()


            g.recording = false;
            g.webSocket.close();
        }

        
    

        function playOpus(roomID){

            var audioStack = [];
            var nextTime = 0;

            if(g.audioContext == null)
                g.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            var opusURL = g.downstreamURL + "/joinRoom?roomID=" + roomID;

            // Fetch a file and decode it.
            fetch(opusURL)
            .then(decodeOpusResponse)
            .then(_ => console.log('decoded '+g.totalSamplesDecoded+' samples.'))
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
                    if(g.playing == false)return;

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

                    var myArrayBuffer = g.audioContext.createBuffer(1,frameCount, sampleRate);

                    var nowBuffering = myArrayBuffer.getChannelData(0);
                    for (var i = 0; i < frameCount; i++) {
                        nowBuffering[i] = buffer[i];
                    }                    

                    var source    = g.audioContext.createBufferSource();
                    source.buffer = myArrayBuffer;
                    source.connect(g.audioContext.destination);
                    if (nextTime == 0)
                        nextTime = g.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like

                    source.start(nextTime);

                    nextTime += source.buffer.duration; // Make the next buffer wait the length of the last buffer before being played
                }
                g.totalSamplesDecoded+=samplesDecoded;
            }
        }
   
        function play(roomID) {

            var audioStack = [];
            var nextTime = 0;
            var leftByte = null

            if(g.audioContext == null)
                g.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            var url= g.downstreamURL + "/joinRoom?format=wav&roomID=" + roomID;

            fetch(url).then(function(response) {

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

                        if(g.playing == false)return

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
                            
                            var myArrayBuffer = g.audioContext.createBuffer(1, frameCount , 48000);
                            var nowBuffering = myArrayBuffer.getChannelData(0);
        
                            for (var i = 0; i < frameCount; i++) {
                                nowBuffering[i] = buffer[i] / 32768.0;
                            }               
                            
                            var source    = g.audioContext.createBufferSource();
                            source.buffer = myArrayBuffer;

                            source.connect(g.audioContext.destination);
                            if (nextTime == 0)
                                nextTime = g.audioContext.currentTime + 0.01;  /// add 50ms latency to work well across systems - tune this if you like
                                
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
            g.playing = false
        }
