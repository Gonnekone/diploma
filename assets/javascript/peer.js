function copyToClipboard(text) {
    if (window.clipboardData && window.clipboardData.setData) {
        clipboardData.setData("Text", text);
        return Swal.fire({
            position: 'top-end',
            text: "Copied",
            showConfirmButton: false,
            timer: 1000,
            width: '150px'
        })
    } else if (document.queryCommandSupported && document.queryCommandSupported("copy")) {
        var textarea = document.createElement("textarea");
        textarea.textContent = text;
        textarea.style.position = "fixed";
        document.body.appendChild(textarea);
        textarea.select();
        try {
            document.execCommand("copy");
            return Swal.fire({
                position: 'top-end',
                text: "Copied",
                showConfirmButton: false,
                timer: 1000,
                width: '150px'
            })
        } catch (ex) {
            console.warn("Copy to clipboard failed.", ex);
            return false;
        } finally {
            document.body.removeChild(textarea);
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    (document.querySelectorAll('.notification .delete') || []).forEach(($delete) => {
        const $notification = $delete.parentNode;
        $delete.addEventListener('click', () => {
            $notification.style.display = 'none'
        });
    });
});

function connect(stream) {
    document.getElementById('peers').style.display = 'block';
    document.getElementById('noperm').style.display = 'none';

    const turnHost = "turn." + window.location.hostname;

    let pc = new RTCPeerConnection({
        iceServers: [
            {urls: "stun:stun.l.google.com:19302"},
            {urls: "turn:" + turnHost + ":3478", username: "gonnekone", credential: "gonnekone"}
        ]
    });

    pc.ontrack = (e) => {
        const s = e.streams && e.streams[0];
        if (!s) return;

        if (window.localStream && s.id === window.localStream.id) {
            return;
        }

        if (e.track.kind !== 'video') return;

        const videos = document.getElementById('videos');
        const id = `remote-${s.id}`;

        let wrapper = document.getElementById(id);
        let el;
        if (!wrapper) {
            wrapper = document.createElement('div');
            wrapper.className = 'column is-6 peer';
            wrapper.id = id;

            el = document.createElement('video');
            el.autoplay = true;
            el.playsInline = true;
            el.controls = true;

            wrapper.appendChild(el);
            document.getElementById('noone').style.display = 'none';
            document.getElementById('nocon').style.display = 'none';
            videos.appendChild(wrapper);

            const tryPlay = setInterval(() => {
                el.play().then(() => clearInterval(tryPlay)).catch(() => {
                });
            }, 1500);
        } else {
            el = wrapper.querySelector('video');
        }

        if (el.srcObject !== s) el.srcObject = s;

        e.track.onmute = () => {
            el.play().catch(() => {
            });
        };

        s.addEventListener('removetrack', () => {
            const node = document.getElementById(id);
            if (node && node.parentNode) node.parentNode.remove();
            if (videos.childElementCount <= 3) {
                document.getElementById('noone').style.display = 'grid';
                document.getElementById('noonein').style.display = 'grid';
            }
        });
    };

    stream.getTracks().forEach(track => pc.addTrack(track, stream));

    let ws = new WebSocket(RoomWebsocketAddr);

    pc.onicecandidate = e => {
        if (!e.candidate) return;
        ws.send(JSON.stringify({
            event: 'candidate',
            data: JSON.stringify(e.candidate)
        }));
    };

    ws.addEventListener('error', (event) => {
        console.log('error: ', event);
    });

    ws.onclose = function () {
        console.log("websocket has closed");
        pc.close();
        pc = null;
        const pr = document.getElementById('videos');
        while (pr.childElementCount > 3) {
            pr.lastChild.remove();
        }
        document.getElementById('noone').style.display = 'none';
        document.getElementById('nocon').style.display = 'flex';
        setTimeout(function () {
            connect(stream);
        }, 1000);
    };

    ws.onmessage = function (evt) {
        const msg = JSON.parse(evt.data);
        if (!msg) {
            return console.log('failed to parse msg');
        }

        switch (msg.event) {
            case 'offer': {
                const offer = JSON.parse(msg.data);
                if (!offer) {
                    return console.log('failed to parse offer');
                }
                pc.setRemoteDescription(offer);
                pc.createAnswer().then(answer => {
                    pc.setLocalDescription(answer);
                    ws.send(JSON.stringify({
                        event: 'answer',
                        data: JSON.stringify(answer)
                    }));
                });
                return;
            }
            case 'candidate': {
                const candidate = JSON.parse(msg.data);
                if (!candidate) {
                    return console.log('failed to parse candidate');
                }
                pc.addIceCandidate(candidate);
                return;
            }
        }
    };

    ws.onerror = function (evt) {
        console.log("error: " + evt.data);
    };
}

function createBlackVideoTrack() {
    const canvas = document.createElement('canvas');
    canvas.width = 320;
    canvas.height = 240;
    canvas.style.display = 'none';
    document.body.appendChild(canvas);

    const ctx = canvas.getContext('2d');
    const drawFrame = () => {
        ctx.fillStyle = 'black';
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        requestAnimationFrame(drawFrame);
    };
    drawFrame();

    const blackStream = canvas.captureStream(30);
    const videoTrack = blackStream.getVideoTracks()[0];

    videoTrack.addEventListener('ended', () => canvas.remove());

    return videoTrack;
}

const audioConstraints = {
    sampleSize: 16,
    channelCount: 2,
    echoCancellation: true
};

async function getLocalStream() {
    try {
        const stream = await navigator.mediaDevices.getUserMedia({
            video: {
                width: { max: 1280 },
                height: { max: 720 },
                aspectRatio: 4 / 3,
                frameRate: 30,
            },
            audio: audioConstraints
        });
        return stream;
    } catch (e) {
        console.log('No camera or both devices missing:', e);
        const stream = new MediaStream();
        stream.addTrack(createBlackVideoTrack());

        try {
            const audioStream = await navigator.mediaDevices.getUserMedia({ audio: audioConstraints });
            audioStream.getAudioTracks().forEach(track => stream.addTrack(track));
        } catch (audioErr) {
            console.log('No microphone either');
        }

        return stream;
    }
}

getLocalStream().then(stream => {
    window.localStream = stream;
    document.getElementById('localVideo').srcObject = stream;
    connect(stream);
}).catch(err => {
    console.error('Unexpected error', err);
    const fallbackStream = new MediaStream();
    fallbackStream.addTrack(createBlackVideoTrack());
    window.localStream = fallbackStream;
    document.getElementById('localVideo').srcObject = fallbackStream;
    connect(fallbackStream);
});