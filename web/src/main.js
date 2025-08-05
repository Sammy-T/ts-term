import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';

const term = new Terminal();
const fitAddon = new FitAddon();

function connectInitWs() {
	const websocket = new WebSocket(`ws://${location.host}/ts`);

	const machineMsg = 'Tailscale machine';

	websocket.onopen = (ev) => {
		term.write('Init WebSocket open.\r\n');
	};

	websocket.onmessage = (ev) => {
		console.log(ev);
		term.write(ev.data);

		if(ev.data.startsWith(machineMsg)) {
			const hostname = ev.data.split(' ').at(2);
			connectTsWs(`ws://${hostname}`)
		}
	};

	websocket.onclose = (ev) => {
		console.log(ev);
		term.write('Init Websocket closed.\r\n');
	};

	websocket.onerror = (ev) => {
		console.log(ev);
		term.write('Init Websocket error.\r\n');
	}

	window.addEventListener('pagehide', () => {
		websocket.close();
	});
}

function connectTsWs(url) {
	const websocket = new WebSocket(url);

	term.onData((data) => {
		websocket.send(data);
	});

	websocket.onopen = (ev) => {
		term.write('Tailscale WebSocket open.\r\n');
	};

	websocket.onmessage = (ev) => {
		console.log(ev);
		term.write(ev.data);
	};

	websocket.onclose = (ev) => {
		console.log(ev);
		term.write('Tailscale Websocket closed.\r\n');
	};

	websocket.onerror = (ev) => {
		console.log(ev);
		term.write('Tailscale Websocket error.\r\n');
	}

	window.addEventListener('pagehide', () => {
		websocket.close();
	});
}

term.loadAddon(fitAddon);
term.open(document.querySelector('#xterm-container'));
fitAddon.fit();

term.write('Hello from \x1B[1;3;31mxterm.js\x1B[0m \r\n');

window.addEventListener('resize', () => {
	fitAddon.fit();
});

connectInitWs();
