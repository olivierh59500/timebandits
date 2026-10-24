package com.olivierh.timebandits;

import android.app.Activity;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;

import com.olivierh.timebandits.mobile.EbitenView;
import com.olivierh.timebandits.mobile.Mobile;

import go.Seq;

/** Android lifecycle bridge; rendering, controls and gameplay remain in Go. */
@SuppressWarnings("deprecation")
public final class MainActivity extends Activity {
    private EbitenView gameView;
    private boolean suspended;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        Seq.setContext(getApplicationContext());
        Mobile.configure(getFilesDir().getAbsolutePath() + "/saves.json");
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            WindowManager.LayoutParams attributes = getWindow().getAttributes();
            attributes.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
            getWindow().setAttributes(attributes);
        }
        gameView = new EbitenView(this);
        gameView.setFocusable(true);
        gameView.setFocusableInTouchMode(true);
        setContentView(gameView);
        gameView.requestFocus();
        hideSystemBars();
        if (Build.VERSION.SDK_INT >= 33) {
            getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT, Mobile::back);
        }
    }

    @Override
    protected void onPause() {
        Mobile.suspend();
        if (gameView != null && !suspended) {
            gameView.suspendGame();
            suspended = true;
        }
        super.onPause();
    }

    @Override
    protected void onResume() {
        super.onResume();
        hideSystemBars();
        if (gameView != null && suspended) {
            gameView.resumeGame();
            suspended = false;
        }
    }

    @Override
    public void onWindowFocusChanged(boolean focused) {
        super.onWindowFocusChanged(focused);
        if (focused) {
            hideSystemBars();
        }
    }

    @Override
    public void onBackPressed() {
        Mobile.back();
    }

    private void hideSystemBars() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
            WindowInsetsController controller = getWindow().getInsetsController();
            if (controller != null) {
                controller.hide(WindowInsets.Type.statusBars() | WindowInsets.Type.navigationBars());
                controller.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
            }
        } else {
            getWindow().getDecorView().setSystemUiVisibility(
                    View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                            | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                            | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_STABLE);
        }
    }
}
