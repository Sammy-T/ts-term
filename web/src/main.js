import ghLogo from './brand-github.svg?raw';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';

const term = new Terminal();
const fitAddon = new FitAddon();

const termContainer = document.querySelector('#xterm-container');
const ghAnchor = document.querySelector('#gh');

/** @type {HTMLDialogElement} */
const dialogConn = document.querySelector('#diag-conn');

/** @type {HTMLDialogElement} */
const dialogProg = document.querySelector('#diag-prog');

/** @type {HTMLDialogElement} */
const dialogErr = document.querySelector('#diag-err');

/** @type {WebSocket} */
let initWs;

/** @type {WebSocket} */
let tsWs;

/** @type {String} */
let tsWsUrl;

function connectInitWs() {
	initWs = new WebSocket(`ws://${location.host}/ts`);

	const machineMsg = 'Tailscale machine';

	initWs.onopen = (ev) => {
		term.write('Init WebSocket open.\r\n');
	};

	initWs.onmessage = (ev) => {
		console.log(ev);
		term.write(ev.data);

		if(ev.data.startsWith(machineMsg)) {
			const proto = (location.protocol === 'https:') ? 'wss:' : 'ws:';
			const hostname = ev.data.split(' ').at(2);

			tsWsUrl = `${proto}//${hostname}`;

			//// TODO: Update ssh dialog machines
			dialogConn.showModal();
		}
	};

	initWs.onclose = (ev) => {
		console.log(ev);
		term.write('Init WebSocket closed.\r\n');
	};

	initWs.onerror = (ev) => {
		console.log(ev);
		term.write('Init WebSocket error.\r\n');
	}

	window.addEventListener('pagehide', () => {
		initWs.close();
	});
}

function connectTsWs(url) {
	tsWs = new WebSocket(url);

	/**
	 * Tracks the newline status of received server data output to the terminal UI.
	 * 
	 * Server output will usually contain a newline but returned PTY output might not.
	 */
	let isOnNewline = true;

	term.onData((data) => {
		const msg = {
			type: 'input',
			data
		};

		tsWs.send(JSON.stringify(msg));
	});

	tsWs.onopen = (ev) => {
		dialogProg.close();

		initWs.send('ts-websocket-opened');
		term.write('Tailscale WebSocket open.\r\n');

		onSize();
	};

	tsWs.onmessage = (ev) => {
		console.log(ev);
		term.write(ev.data);

		isOnNewline = ev.data.endsWith('\r\n');
	};

	tsWs.onclose = (ev) => {
		console.log(ev);

		const msg = 'Tailscale WebSocket closed.\r\n';
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	};

	tsWs.onerror = (ev) => {
		console.log(ev);

		dialogProg.close();
		// if(initWs.readyState === WebSocket.OPEN) dialogErr.showModal(); //// TODO: err/retry dialog?

		initWs.send('ts-websocket-error');

		const msg = 'Tailscale WebSocket error.\r\n';
		term.write((isOnNewline) ? msg : `\r\n${msg}`);
	}

	window.addEventListener('pagehide', () => {
		tsWs.close();
	});
}

function onSize() {
	const { rows, cols } = term;
	const { clientWidth, clientHeight } = termScreen;
	console.log(`rows: ${rows}, cols: ${cols}, width: ${clientWidth}, height: ${clientHeight}`);

	if(!tsWs) return;

	const msg = {
		type: 'size',
		data: JSON.stringify({ rows, cols, x: clientWidth, y: clientHeight }),
	};

	tsWs.send(JSON.stringify(msg))
}

function initDialogs() {
	dialogConn.querySelector('#config').addEventListener('submit', async (ev) => {
		dialogProg.showModal();

		const formData = new FormData(ev.target);

		const address = formData.get('address');
		const port = formData.get('port');
		const username = formData.get('username');
		const password = formData.get('password');

		initWs.send(`ssh-config:${username}:${password}:${address}:${port}`);

		setTimeout(() => connectTsWs(tsWsUrl), 1000);
	});

	dialogErr.querySelector('form').addEventListener('submit', (ev) => {
		dialogConn.showModal();
	});
}

// Add the GH logo into the footer link
ghAnchor.innerHTML = `${ghLogo} ${ghAnchor.innerHTML}`;

term.loadAddon(fitAddon);
term.loadAddon(new WebLinksAddon());
term.open(termContainer);
fitAddon.fit();

const termScreen = termContainer.querySelector('.xterm-screen');

/** @type {Number} */
let tid;

term.onResize(({ rows, cols }) => {
	clearTimeout(tid);
	tid = setTimeout(() => onSize(), 500);
});

const rsObserver = new ResizeObserver(() => {
	fitAddon.fit();
});

rsObserver.observe(termContainer);

term.write('Welcome to \x1B[1;3;32mts-term\x1B[0m \r\n');

initDialogs();
connectInitWs();
