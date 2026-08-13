package receive

// toneMapShader rolls a PQ picture down into the range a standard display shows.
//
// It runs where the frames are still coded as they arrived: glcolorconvert ahead of it
// applies matrix and range and no transfer function, so what reaches the sampler is
// non-linear PQ in BT.2020 primaries, which is exactly what the curve below inverts.
//
// Five steps, and the middle one is the whole point.
//
//  1. The ST 2084 EOTF turns the signal into light, on the scale the format defines, where
//     1.0 is ten thousand candela per square metre.
//  2. Dividing by BT.2408 reference white puts diffuse white at 1.0. That is the alignment
//     the picture is judged by: a desktop's white is reference white, and a map that left it
//     anywhere else would be the darkened picture videoconvert's gamma-mode already
//     produces.
//  3. Everything above the knee is rolled into what is left below 1.0, on luminance rather
//     than per channel, so a highlight loses brightness and not its hue.
//  4. BT.2020 primaries become BT.709.
//  5. The sRGB curve encodes the result.
//
// The shoulder is an exponential rather than the Hermite spline BT.2390 states, and the
// reason is the range it has to cover. BT.2390 rolls off inside the PQ domain, where the
// whole range is [0,1] and the slopes either side of the knee are near one. Once reference
// white is 1.0 the range above it runs to about fifty, and a Hermite segment that leaves the
// knee at unit slope and has to arrive at 1.0 that far away overshoots and comes back down,
// which is a curve that makes a brighter input darker. The exponential leaves the knee at
// unit slope, never exceeds 1.0 and never turns over, so highlights compress into the top of
// the range in the order they came in.
//
// What it gives up is the highlights themselves, and that is a property of the display
// rather than of this curve. A standard display has no room above diffuse white, so specular
// detail above the knee arrives compressed into a narrow band. Rolling it off smoothly is
// what separates this from clipping, which is the alternative.
const toneMapShader = `
#ifdef GL_ES
precision highp float;
#endif

varying vec2 v_texcoord;
uniform sampler2D tex;

// SMPTE ST 2084, table 4.
const float pqM1 = 0.1593017578125;
const float pqM2 = 78.84375;
const float pqC1 = 0.8359375;
const float pqC2 = 18.8515625;
const float pqC3 = 18.6875;

// BT.2408 reference white, 203 cd/m^2, as the fraction of full scale PQ codes it.
const float refWhite = 0.0203;

// Where the roll-off starts, once reference white is 1.0. Below it the picture passes
// through untouched, which is where a desktop's own content sits.
const float knee = 0.85;

const vec3 lumaBt2020 = vec3(0.2627, 0.6780, 0.0593);

// BT.2020 to BT.709, column major.
const mat3 bt2020ToBt709 = mat3(
     1.6605, -0.1246, -0.0182,
    -0.5876,  1.1329, -0.1006,
    -0.0728, -0.0083,  1.1187);

// The ST 2084 EOTF. The denominator is floored because the numerator reaches zero with it
// and the ratio is raised to a power.
vec3 pqToLinear(vec3 signal) {
    vec3 p = pow(clamp(signal, 0.0, 1.0), vec3(1.0 / pqM2));
    return pow(max(p - pqC1, 0.0) / max(pqC2 - pqC3 * p, 1e-6), vec3(1.0 / pqM1));
}

// The shoulder. Value and slope both carry across the knee, and the result approaches 1.0
// without reaching it, so the curve never turns over however bright the input is.
float shoulder(float y) {
    if (y <= knee) {
        return y;
    }
    return knee + (1.0 - knee) * (1.0 - exp(-(y - knee) / (1.0 - knee)));
}

vec3 linearToSrgb(vec3 c) {
    c = clamp(c, 0.0, 1.0);
    vec3 low = c * 12.92;
    vec3 high = 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055;
    return mix(low, high, step(vec3(0.0031308), c));
}

void main() {
    vec4 texel = texture2D(tex, v_texcoord);

    vec3 light = pqToLinear(texel.rgb) / refWhite;

    float luma = dot(light, lumaBt2020);
    light *= luma > 0.0 ? shoulder(luma) / luma : 0.0;

    gl_FragColor = vec4(linearToSrgb(bt2020ToBt709 * light), texel.a);
}
`

// glToneMapRung is the rung every platform carries, and the only one that brings its own
// conversion rather than asking a driver for one.
//
// It is last in every list because it is the one that always builds: OpenGL is where the
// default render chain already works on Linux, and a driver rung ahead of it converts on
// silicon that is built for it.
//
// The fragment ends in gldownload, which is what lets one rung serve every chain. gldownload
// passes GL memory straight through when what follows accepts it, so the GL chain pays
// nothing and its own glupload finds the frames already uploaded; a chain that works in
// system memory or on another device gets the download it needs to take them.
//
// glcolorconvert ahead of the shader is not optional. glshader samples one RGBA texture, and
// a decoder hands over planar YUV, so without it there is nothing for the sampler to read.
// It applies matrix and range and no transfer function, which is what leaves the PQ curve
// intact for the shader to invert.
var glToneMapRung = toneMapRung{
	name:  "gl",
	needs: []string{"glupload", "glcolorconvert", "glshader", "gldownload"},
	elements: []string{
		"glupload",
		"glcolorconvert",
		"video/x-raw(" + glMemory + "),format=RGBA",
		"glshader name=" + toneMapName,
		"gldownload",
	},
	shader: toneMapShader,
}
