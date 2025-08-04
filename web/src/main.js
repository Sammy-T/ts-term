import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';

// const websocket = new WebSocket(`ws://${location.host}/term`);
const websocket = new WebSocket(`ws://${location.host}/ts`);

const term = new Terminal();
const fitAddon = new FitAddon();

term.loadAddon(fitAddon);
term.open(document.querySelector('#xterm-container'));
fitAddon.fit();

term.write('Hello from \x1B[1;3;31mxterm.js\x1B[0m \r\n');

term.onData((data) => {
	websocket.send(data);
});

websocket.onopen = (ev) => {
	term.write('WebSocket open.\r\n');
};

websocket.onmessage = (ev) => {
	console.log(ev);
	term.write(ev.data);
};

websocket.onclose = (ev) => {
	console.log(ev);
	term.write('Websocket closed.\r\n');
};

window.addEventListener('resize', () => {
	fitAddon.fit();
});

window.addEventListener('pagehide', () => {
	websocket.close();
});
