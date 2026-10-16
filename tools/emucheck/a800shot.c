/* a800shot: run atari800's emulator core (libatari800, the same code as the
   atari800 program) without a window for a number of frames, then write what
   is on the screen: 384x240 bytes of GTIA colour values (the screen buffer,
   one byte a pixel) followed by atari800's 256-colour palette as 256 x 3
   bytes RGB. Used by Charcoal's emulator check (emu_test.go).

   usage: a800shot FRAMES OUT [atari800 options...] FILE
*/
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "libatari800.h"

extern int Colours_table[256];

int main(int argc, char **argv)
{
	input_template_t input;
	int frames, i;
	FILE *f;

	if (argc < 4) {
		fprintf(stderr, "usage: a800shot FRAMES OUT [atari800 options...] FILE\n");
		return 2;
	}
	frames = atoi(argv[1]);
	/* the options after OUT go to atari800 as on its command line */
	if (!libatari800_init(argc - 3, argv + 3)) {
		fprintf(stderr, "a800shot: %s\n", libatari800_error_message());
		return 1;
	}
	libatari800_clear_input_array(&input);
	for (i = 0; i < frames; i++)
		libatari800_next_frame(&input);
	f = fopen(argv[2], "wb");
	if (!f) {
		perror(argv[2]);
		return 1;
	}
	fwrite(libatari800_get_screen_ptr(), 1, 384 * 240, f);
	for (i = 0; i < 256; i++) {
		fputc((Colours_table[i] >> 16) & 0xff, f);
		fputc((Colours_table[i] >> 8) & 0xff, f);
		fputc(Colours_table[i] & 0xff, f);
	}
	fclose(f);
	libatari800_exit();
	return 0;
}
