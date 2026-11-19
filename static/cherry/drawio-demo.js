// Extends EditorUi to update I/O action states based on availability of backend
(function () {
  var editorUiInit = EditorUi.prototype.init;

  EditorUi.prototype.init = function () {
    editorUiInit.apply(this, arguments);
  };

  // Adds required resources (disables loading of fallback properties, this can only
  // be used if we know that all keys are defined in the language specific file)
  mxResources.loadDefaultBundle = false;
  var bundle = mxResources.getDefaultBundle(mxLanguage);

  // Fixes possible asynchronous requests
  mxUtils.getAll([bundle, '/static/cherry/drawio_demo/default.xml'], function (xhr) {
    // Adds bundle text to resources
    mxResources.parse(xhr[0].getText());

    // Configures the default graph theme
    var themes = new Object();
    themes[Graph.prototype.defaultThemeName] = xhr[1].getDocumentElement();

    // Main
    window.editorUIInstance = new EditorUi(new Editor(false, themes));

    try {
      addPostMessageListener(editorUIInstance.editor);
    } catch (error) {
      console.log(error);
    }
    window.parent.postMessage({ eventName: 'ready', value: '' }, '*');

  }, function () {
    document.body.innerHTML = '<center style="margin-top:10%;">Error loading resource files. Please check browser console.</center>';
  });
})();

/**
 * MinDoc补丁: 公共 XML 解析函数。
 * 兼容: ① encodeURI / encodeURIComponent 编码的 XML；② <mxfile> 包装（diagrams.net 的 .drawio 文件、
 * 编辑器旧数据），diagram 内容为 base64(deflateRaw(encodeURIComponent(xml)))，解包出内层 mxGraphModel。
 * 返回可直接 importGraphModel/setGraphXml 的 mxGraphModel 元素。
 */
window.mindocParseGraphXml = function (xmlText) {
  // v=2026011902 兼容 drawio 桌面版未压缩格式
  try { xmlText = decodeURIComponent(xmlText); } catch (decodeErr1) {
    try { xmlText = decodeURI(xmlText); } catch (decodeErr2) {}
  }
  var doc = mxUtils.parseXml(xmlText);
  var root = doc.documentElement;
  if (root.nodeName === 'mxfile') {
    // 情况1: <mxfile><mxGraphModel>...</mxGraphModel></mxfile>（无 diagram 包装的未压缩格式）
    var directModel = root.getElementsByTagName('mxGraphModel')[0];
    if (directModel) {
      root = directModel;
    } else {
      var diagramEl = root.getElementsByTagName('diagram')[0];
      if (diagramEl) {
        // 情况2: <diagram> 内含 <mxGraphModel> 子元素（drawio 桌面版 Electron 默认的未压缩保存格式）
        var innerEl = diagramEl.getElementsByTagName('mxGraphModel')[0];
        if (innerEl) {
          root = innerEl;
        } else {
          // 情况3: 压缩格式，diagram 文本内容为 base64(deflateRaw(encodeURIComponent(xml)))，解压后仍是 URL 编码，需再解码
          var payload = (diagramEl.textContent || '').trim();
          if (payload) {
            var innerXml = '';
            if (payload.indexOf('%3C') === 0) {
              innerXml = decodeURIComponent(payload);
            } else if (payload.indexOf('<') === 0) {
              innerXml = payload;
            } else {
              var b64 = payload.replace(/-/g, '+').replace(/_/g, '/').replace(/\s/g, '');
              b64 += '='.repeat((4 - b64.length % 4) % 4);
              var bin = atob(b64);
              var bytes = new Uint8Array(bin.length);
              for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
              innerXml = pako.inflateRaw(bytes, { to: 'string' });
              if (innerXml.indexOf('<') !== 0) {
                try { innerXml = decodeURIComponent(innerXml); } catch (innerDecodeErr) {}
              }
            }
            doc = mxUtils.parseXml(innerXml);
            root = doc.documentElement;
          }
        }
      }
    }
  }
  return root;
};

function addPostMessageListener(graphEditor) {
  window.addEventListener('message', function (event) {
    if (!event.data || !event.data.eventName) {
      return
    }
    switch (event.data.eventName) {
      case 'setData':
        var root = window.mindocParseGraphXml(event.data.value);
        var documentName = 'cherry-drawio-' + new Date().getTime();
        editorUIInstance.editor.setGraphXml(null);
        graphEditor.graph.importGraphModel(root);
        graphEditor.setFilename(documentName);
        window.parent.postMessage({ eventName: 'setData:success', value: '' }, '*');
        break;
      case 'getData':
        editorUIInstance.editor.graph.stopEditing();
        var xmlData = mxUtils.getXml(editorUIInstance.editor.getGraphXml());
        // MinDoc补丁(v=2026011903): EditorUi.js 中 handleError 被覆盖为直接 rethrow，
        // 图形含外链图片时 canvas 被跨域污染、toDataURL 抛异常且无回调，点击确定无反应。
        // 处理：接管错误 → 失败则临时移除外链图片样式重新导出（xmlData 保留原图样式，不影响重新编辑）→ 导出后恢复样式
        var getDataOrigHandleError = editorUIInstance.handleError;
        var getDataFinished = false;
        var getDataFinish = function (base64) {
          editorUIInstance.handleError = getDataOrigHandleError;
          if (!getDataFinished) {
            getDataFinished = true;
            window.parent.postMessage({
              mceAction: 'getData:success',
              eventName: 'getData:success',
              value: {
                xmlData: xmlData,
                base64: base64,
              }
            }, '*');
          }
        };
        var getDataExport = function (onDone, onFail) {
          editorUIInstance.handleError = function (e) { onFail(e); };
          editorUIInstance.exportImage(2, "#ffffff", true, null, true, 50, null, "png", function (base64, filename) {
            onDone(base64);
          });
        };
        var getDataRestoreStyles = function (oldStyles) {
          var model = editorUIInstance.editor.graph.getModel();
          model.beginUpdate();
          try {
            for (var i = 0; i < oldStyles.length; i++) {
              model.setStyle(oldStyles[i][0], oldStyles[i][1]);
            }
          } finally {
            model.endUpdate();
          }
        };
        getDataExport(function (base64) {
          getDataFinish(base64);
        }, function () {
          // 降级：移除外链图片样式重新导出
          var model = editorUIInstance.editor.graph.getModel();
          var oldStyles = [];
          model.beginUpdate();
          try {
            var cells = model.cells || {};
            for (var id in cells) {
              var cell = cells[id];
              var style = (cell && cell.getStyle) ? cell.getStyle() : null;
              if (style && style.indexOf('image=http') >= 0) {
                oldStyles.push([cell, style]);
                model.setStyle(cell, style.replace(/image=[^;]*;?/, ''));
              }
            }
          } finally {
            model.endUpdate();
          }
          getDataExport(function (base64) {
            getDataRestoreStyles(oldStyles);
            getDataFinish(base64);
          }, function () {
            getDataRestoreStyles(oldStyles);
            getDataFinish('');
          });
        });
        break;
      case 'ready?':
        window.parent.postMessage({ eventName: 'ready', value: '' }, '*');
        break;
      default:
        break;
    }
  });
}
