import ghLogo from './brand-github.svg?raw';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';

/**
 * @typedef {Object} WsMessage
 * @property {String} type
 * @property {String} data
 */

const term = new Terminal();
const fitAddon = new FitAddon();

const termContainer = document.querySelector('#xterm-container');
const ghAnchor = document.querySelector('#gh');

/** @type {HTMLDialogElement} */
const dialogConn = document.querySelector('#diag-conn');

/** @type {HTMLDialogElement} */
const dialogProg = document.querySelector('#diag-prog');

/** @type {HTMLDialogElement} */
const dialogHosts = document.querySelector('#diag-host-prompt');

/** @type {HTMLDialogElement} */
const dialogErr = document.querySelector('#diag-err');

/** @type {HTMLFormElement} */
const settingsForm = dialogConn.querySelector('#machine-settings');

/** @type {HTMLFormElement} */
const configForm = dialogConn.querySelector('#config');

/** @type {HTMLSelectElement} */
const machineSelect = settingsForm.querySelector('select[name="machine"]');

/** @type {HTMLSelectElement} */
const typeSelect = settingsForm.querySelector('select[name="address-type"]');

/** @type {WebSocket} */
let initWs;

/** @type {WebSocket} */
let tsWs;

/** @type {String} */
let tsWsUrl;

/** @type {Array} */
let peerInfos;

const proto = (location.protocol === 'https:') ? 'wss:' : 'ws:';

/**
 * Tracks the newline status of received server data output to the terminal UI.
 * 
 * Server output will usually contain a newline but returned PTY output might not.
 */
let isOnNewline = true;

function connectInitWs() {
	initWs = new WebSocket(`${proto}//${location.host}/ts`);

	const machineMsg = 'Tailscale machine';

	initWs.onopen = (ev) => {
		term.write('Init WebSocket open.\r\n');
		isOnNewline = true;
	};

	initWs.onmessage = (ev) => {
		console.log(ev);

		/** @type {WsMessage} */
		const msg = JSON.parse(ev.data);

		switch(msg.type) {
			case 'info':
				if(msg.data.startsWith(machineMsg)) {
					const hostname = ev.data.split(' ').at(2);
					tsWsUrl = `${proto}//${hostname}`;
				}
				break;

			case 'peers':
				const infos = JSON.parse(msg.data);
				peerInfos = infos.sort((a, b) => a.shortDomain.localeCompare(b.shortDomain));
				
				updateMachines();
				dialogConn.showModal();
				return
			
			default:
				return
		}

		term.write(`${msg.data}\r\n`);
		isOnNewline = true;
	};

	initWs.onerror = (ev) => {
		dialogProg.close();

		console.log(ev);
		term.write('Init WebSocket error.\r\n');

		isOnNewline = true;
	}

	initWs.onclose = (ev) => {
		dialogProg.close();
		dialogConn.close();

		console.log(ev);
		const msg = `Init WebSocket closed. ${ev.reason || ''}\r\n`;
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	};

	window.addEventListener('pagehide', () => {
		initWs.close();
	});
}

function connectTsWs(url) {
	tsWs = new WebSocket(url);

	term.onData((data) => {
		/** @type {WsMessage} */
		const msg = {
			type: 'input',
			data
		};

		tsWs.send(JSON.stringify(msg));
	});

	tsWs.onopen = (ev) => {
		dialogProg.close();

		/** @type {WsMessage} */
		const msg = {
			type: 'ts-websocket-opened',
		};

		initWs.send(JSON.stringify(msg));
		term.write('Tailscale WebSocket open.\r\n');

		isOnNewline = true;
	};

	tsWs.onmessage = (ev) => {
		console.log(ev);

		dialogProg.close();

		/** @type {WsMessage} */
		const msg = JSON.parse(ev.data);

		switch(msg.type) {
			case 'ssh-error':
				dialogErr.showModal();
				return;
			case 'ssh-host':
				const host = dialogHosts.querySelector('#host');
				host.innerHTML = msg.data;

				dialogHosts.showModal();
				return;
			case 'ssh-success':
				onSize();
				return;
			case 'info':
				term.write(msg.data + '\r\n');
				isOnNewline = true;
				break;
			case 'output':
				term.write(msg.data);
				isOnNewline = msg.data.endsWith('\r\n');
				break;
		}
	};

	tsWs.onerror = (ev) => {
		console.log(ev);

		dialogProg.close();

		/** @type {WsMessage} */
		const wsMsg = {
			type: 'ts-websocket-error',
		};

		initWs.send(JSON.stringify(wsMsg));

		const msg = 'Tailscale WebSocket error.\r\n';
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	}

	tsWs.onclose = (ev) => {
		console.log(ev);

		dialogProg.close();
		dialogErr.close();

		const msg = `Tailscale WebSocket closed. ${ev.reason || ''}\r\n`;
		term.write((isOnNewline) ? msg : `\r\n${msg}`);

		isOnNewline = true;
	};

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

	tsWs.send(JSON.stringify(msg));
}

function updateMachines() {
	let machineOpts = `<option value="">-- machines --</option>\n`;

	peerInfos.forEach((info, i) => {
		const { shortDomain, ips } = info;
		machineOpts += `<option value="${i}">${shortDomain} [${ips[0]}]</option>\n`;
	});

	machineSelect.innerHTML = machineOpts;
}

/**
 * @param {InputEvent} event 
 */
function onMachineSelect(event) {
	const formData = new FormData(settingsForm);

	const machineIdx = formData.get('machine');
	const type = formData.get('address-type');

	if(machineIdx === '') return;

	const info = peerInfos[Number(machineIdx)];

	let address = info.shortDomain;

	switch(type) {
		case 'domain':
			address = info.shortDomain;
			break;

		case 'full':
			address = info.domain;
			break;

		case 'ip':
			address = info.ips[0];
			break;
	}

	configForm.querySelector('input[name="address"]').value = address;
}

/**
 * @param {Element} container 
 * @param {KeyboardEvent} event 
 */
function handleActionKey(container, event) {
	const buttons = Array.from(container.querySelectorAll('button'));
	const focusedIdx = buttons.findIndex((button) => button === document.activeElement);

	let nextFocusIdx = 0;

	switch(event.key) {
		case 'ArrowLeft':
			nextFocusIdx = (focusedIdx - 1 >= 0) ? focusedIdx - 1 : buttons.length - 1;
			break;
		
		case 'ArrowRight':
			nextFocusIdx = (focusedIdx + 1) % buttons.length;
			break;

		default:
			return
	}

	buttons[nextFocusIdx].focus();
}

function initDialogs() {
	document.querySelectorAll('.actions').forEach((container) => {
		container.addEventListener('keydown', (event) => handleActionKey(container, event));
	});

	machineSelect.addEventListener('input', onMachineSelect);
	typeSelect.addEventListener('input', onMachineSelect);

	configForm.addEventListener('submit', async (ev) => {
		dialogProg.showModal();

		const formData = new FormData(ev.target);

		const address = formData.get('address');
		const port = formData.get('port');
		const username = formData.get('username');
		const password = formData.get('password');

		const sshMsg = `${username}:${password}:${address}:${port}`;

		/** @type {WsMessage} */
		const msg = {
			type: 'ssh-config',
			data: sshMsg,
		};

		const msgStr = JSON.stringify(msg);

		if(!tsWs) {
			initWs.send(msgStr);
			
			// Attempt connection to the ts websocket after a delay
			setTimeout(() => connectTsWs(tsWsUrl), 1000);
			return;
		}

		tsWs.send(msgStr);
	});

	dialogHosts.querySelector('form').addEventListener('submit', (ev) => {
		dialogProg.showModal();

		let action = (ev.submitter.name === 'cancel') ? 'no' : 'yes';
		
		/** @type {WsMessage} */
		const wsMsg = {
			type: 'ssh-host-action',
			data: action,
		};

		tsWs.send(JSON.stringify(wsMsg));
	});

	dialogErr.querySelector('form').addEventListener('submit', (ev) => {
		if(ev.submitter.name === 'cancel') {
			/** @type {WsMessage} */
			const wsMsg = {
				type: 'ts-websocket-error',
			};

			(tsWs ?? initWs)?.send(JSON.stringify(wsMsg));
			
			const msg = 'Tailscale WebSocket error.\r\n';
			term.write((isOnNewline) ? msg : `\r\n${msg}`);
			return;
		}
		
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
