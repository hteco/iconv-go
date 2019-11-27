package iconv

/*
#cgo darwin LDFLAGS: -liconv
#cgo freebsd LDFLAGS: -liconv
#cgo windows LDFLAGS: -liconv
#cgo linux LDFLAGS: -liconv
#include <stdlib.h>
#include <iconv.h>

// As of GO 1.6 passing a pointer to Go pointer, will lead to panic
// Therofore we use this wrapper function, to avoid passing **char directly from go
size_t call_iconv(iconv_t ctx, char *in, size_t *size_in, char *out, size_t *size_out){
	return iconv(ctx, &in, size_in, &out, size_out);
}

*/
import "C"
import "syscall"
import "unsafe"

type Converter struct {
	context C.iconv_t
	open    bool
}

// Initialize a new Converter. If fromEncoding or toEncoding are not supported by
// iconv then an EINVAL error will be returned. An ENOMEM error maybe returned if
// there is not enough memory to initialize an iconv descriptor
func NewConverter(fromEncoding string, toEncoding string) (converter *Converter, err error) {
	converter = new(Converter)

	// convert to C strings
	toEncodingC := C.CString(toEncoding)
	fromEncodingC := C.CString(fromEncoding)

	// open an iconv descriptor
	converter.context, err = C.iconv_open(toEncodingC, fromEncodingC)

	// free the C Strings
	C.free(unsafe.Pointer(toEncodingC))
	C.free(unsafe.Pointer(fromEncodingC))

	// check err
	if err == nil {
		// no error, mark the context as open
		converter.open = true
	}

	return
}

// destroy is called during garbage collection
func (this *Converter) destroy() {
	this.Close()
}

// Close a Converter's iconv description explicitly
func (this *Converter) Close() (err error) {
	if this.open {
		_, err = C.iconv_close(this.context)
	}

	return
}

// Convert bytes from an input byte slice into a give output byte slice
//
// As many bytes that can converted and fit into the size of output will be
// processed and the number of bytes read for input as well as the number of
// bytes written to output will be returned. If not all converted bytes can fit
// into output and E2BIG error will also be returned. If input contains an invalid
// sequence of bytes for the Converter's fromEncoding an EILSEQ error will be returned
//
// For shift based output encodings, any end shift byte sequences can be generated by
// passing a 0 length byte slice as input. Also passing a 0 length byte slice for output
// will simply reset the iconv descriptor shift state without writing any bytes.
func (this *Converter) Convert(input []byte, output []byte) (bytesRead int, bytesWritten int, err error) {
	// make sure we are still open
	if this.open {
		inputLeft := C.size_t(len(input))
		outputLeft := C.size_t(len(output))

		if inputLeft > 0 && outputLeft > 0 {
			// we have to give iconv a pointer to a pointer of the underlying
			// storage of each byte slice - so far this is the simplest
			// way i've found to do that in Go, but it seems ugly
			inputPointer := (*C.char)(unsafe.Pointer(&input[0]))
			outputPointer := (*C.char)(unsafe.Pointer(&output[0]))

			_, err = C.call_iconv(this.context, inputPointer, &inputLeft, outputPointer, &outputLeft)

			// update byte counters
			bytesRead = len(input) - int(inputLeft)
			bytesWritten = len(output) - int(outputLeft)
		} else if inputLeft == 0 && outputLeft > 0 {
			// inputPointer will be nil, outputPointer is generated as above
			outputPointer := (*C.char)(unsafe.Pointer(&output[0]))

			_, err = C.call_iconv(this.context, nil, &inputLeft, outputPointer, &outputLeft)

			// update write byte counter
			bytesWritten = len(output) - int(outputLeft)
		} else {
			// both input and output are zero length, do a shift state reset
			_, err = C.call_iconv(this.context, nil, &inputLeft, nil, &outputLeft)
		}
	} else {
		err = syscall.EBADF
	}

	return bytesRead, bytesWritten, err
}

// Convert an input string
//
// EILSEQ error may be returned if input contains invalid bytes for the
// Converter's fromEncoding.
func (this *Converter) ConvertString(input string) (output string, err error) {
	// make sure we are still open
	if this.open {
		// construct the buffers
		inputBuffer := []byte(input)
		outputBuffer := make([]byte, len(inputBuffer)*2) // we use a larger buffer to help avoid resizing later

		// call Convert until all input bytes are read or an error occurs
		var bytesRead, totalBytesRead, bytesWritten, totalBytesWritten int

		for totalBytesRead < len(inputBuffer) && err == nil {
			// use the totals to create buffer slices
			bytesRead, bytesWritten, err = this.Convert(inputBuffer[totalBytesRead:], outputBuffer[totalBytesWritten:])

			totalBytesRead += bytesRead
			totalBytesWritten += bytesWritten

			// check for the E2BIG error specifically, we can add to the output
			// buffer to correct for it and then continue
			if err == syscall.E2BIG {
				// increase the size of the output buffer by another input length
				// first, create a new buffer
				tempBuffer := make([]byte, len(outputBuffer)+len(inputBuffer))

				// copy the existing data
				copy(tempBuffer, outputBuffer)

				// switch the buffers
				outputBuffer = tempBuffer

				// forget the error
				err = nil
			}
		}

		if err == nil {
			// perform a final shift state reset
			_, bytesWritten, err = this.Convert([]byte{}, outputBuffer[totalBytesWritten:])

			// update total count
			totalBytesWritten += bytesWritten
		}

		// construct the final output string
		output = string(outputBuffer[:totalBytesWritten])
	} else {
		err = syscall.EBADF
	}

	return output, err
}
