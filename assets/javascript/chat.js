let chatWs = null;

document.addEventListener('DOMContentLoaded', () => {
    const msg = document.getElementById("msg");
    const log = document.getElementById("log");
    const scroller = document.querySelector('#chat .body');
    const form = document.getElementById("form");

    function currentTime() {
        const d = new Date();
        const hh = String(d.getHours()).padStart(2, '0');
        const mm = String(d.getMinutes()).padStart(2, '0');
        return `${hh}:${mm}`;
    }

    function appendLine(text) {
        const item = document.createElement("div");
        item.innerText = text;
        log.appendChild(item);
        scroller.scrollTop = scroller.scrollHeight; // вровень с последним сообщением
    }

    form.addEventListener('submit', (e) => {
        e.preventDefault();
        if (!chatWs || chatWs.readyState !== WebSocket.OPEN) return;
        const text = msg.value.trim();
        if (!text) return;
        chatWs.send(text);
        msg.value = "";
    });

    function connectChat() {
        chatWs = new WebSocket(ChatWebsocketAddr);

        chatWs.onclose = () => {
            setTimeout(connectChat, 1000);
        };

        chatWs.onmessage = (evt) => {
            const messages = evt.data.split('\n');
            for (const m of messages) {
                appendLine(`${currentTime()} - ${m}`);
            }
        };

        chatWs.onerror = (evt) => {
            console.log("error:", evt);
        };
    }

    connectChat();
});