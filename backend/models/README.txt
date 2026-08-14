Placeholder for the MagFace ONNX model.

This directory should contain:
    magface_iresnet50.onnx  (~170 MB)

If the model is not present, the app still works but quality_score will
be omitted from face detection results. Set FACE_QUALITY_MODEL_PATH in
.env to point elsewhere if you keep the model outside this directory.

See MagFace: https://github.com/IrvingMeng/MagFace
