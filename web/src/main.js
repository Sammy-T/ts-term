import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';

const term = new Terminal();
const fitAddon = new FitAddon();

term.loadAddon(fitAddon);
term.open(document.querySelector('#xterm-container'));
fitAddon.fit();

window.addEventListener('resize', () => {
	fitAddon.fit();
});

term.write('Hello from \x1B[1;3;31mxterm.js\x1B[0m \r\n$ ');

term.onKey(({key, domEvent}) => {
	term.write(key);

	if(key === '\r') {
		term.write('\n$ ');
	}
});
