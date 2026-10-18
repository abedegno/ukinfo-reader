/* Narrow InstallShield helper. All destinations are supplied by the owning
 * importer; paths from a cabinet are never used as output paths. MIT licensed. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "libunshield.h"
int main(int argc, char **argv) {
    if (argc != 3 && argc != 5) return 2;
    Unshield *cab = unshield_open(argv[2]);
    if (!cab) return 3;
    int result = 0;
    int count = unshield_file_count(cab);
    if (count < 0 || count > 100000) { unshield_close(cab); return 4; }
    if (!strcmp(argv[1], "list") && argc == 3) {
        for (int i = 0; i < count; i++) {
            const char *name = unshield_file_name(cab, i);
            if (unshield_file_is_valid(cab, i) && name && !strpbrk(name, "\t\r\n"))
                printf("%d\t%zu\t%s\n", i, unshield_file_size(cab, i), name);
        }
    } else if (!strcmp(argv[1], "save") && argc == 5) {
        char *end;
        long index = strtol(argv[3], &end, 10);
        if (*end || index < 0 || index >= count || !unshield_file_is_valid(cab, (int)index)) result = 5;
        else if (!unshield_file_save(cab, (int)index, argv[4])) result = 6;
    } else result = 2;
    unshield_close(cab);
    return result;
}
