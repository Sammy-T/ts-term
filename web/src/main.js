import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';

const websocket = new WebSocket(`ws://${location.host}/term`);

const term = new Terminal();
const fitAddon = new FitAddon();

term.loadAddon(fitAddon);
term.open(document.querySelector('#xterm-container'));
fitAddon.fit();

term.write('Hello from \x1B[1;3;31mxterm.js\x1B[0m \r\n$ ');

term.onData((data) => {
	websocket.send(data);

	//// TODO: TEMP
	term.write(data);
	
	if(data === '\r') {
		term.write('\n$ ');
	}
	////
});

websocket.onopen = (ev) => {
	term.write('\b\bWebSocket open. \r\n$ ');
};

window.addEventListener('resize', () => {
	fitAddon.fit();
});

window.addEventListener('unload', () => {
	websocket.close();
});
