import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';

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
		term.write('Init WebSocket closed.\r\n');
	};

	websocket.onerror = (ev) => {
		console.log(ev);
		term.write('Init WebSocket error.\r\n');
	}

	window.addEventListener('pagehide', () => {
		websocket.close();
	});
}

function connectTsWs(url) {
	const websocket = new WebSocket(url);

	/**
	 * Tracks the newline status of received server data output to the terminal UI.
	 * 
	 * Server output will usually contain a newline but returned PTY output might not.
	 */
	let isOnNewline = true;

	term.onData((data) => {
		websocket.send(data);
	});

	websocket.onopen = (ev) => {
		term.write('Tailscale WebSocket open.\r\n');
	};

	websocket.onmessage = (ev) => {
		console.log(ev);
		term.write(ev.data);

		isOnNewline = ev.data.endsWith('\n');
	};

	websocket.onclose = (ev) => {
		console.log(ev);

		const msg = 'Tailscale WebSocket closed.\r\n';
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	};

	websocket.onerror = (ev) => {
		console.log(ev);

		const msg = 'Tailscale WebSocket error.\r\n';
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	}

	window.addEventListener('pagehide', () => {
		websocket.close();
	});
}

term.loadAddon(fitAddon);
term.loadAddon(new WebLinksAddon());
term.open(document.querySelector('#xterm-container'));
fitAddon.fit();

term.write('Welcome to \x1B[1;3;32mts-term\x1B[0m \r\n');

window.addEventListener('resize', () => {
	fitAddon.fit();
});

connectInitWs();
